package ultrahdr

import (
	"image"
	"math"
	"runtime"
	"sync"
)

type resampleWeights struct {
	coeffs       []float32
	start        []int
	filterLength int
}

type kernelDef struct {
	interp Interpolation
	taps   int
	kernel func(float64) float64
}

type weightsKey struct {
	src    int
	dst    int
	interp Interpolation
}

var weightsCache sync.Map

var float32Pool = sync.Pool{
	New: func() any {
		buf := make([]float32, 0)
		return &buf
	},
}

var (
	maxParallelWorkers = 0
	workerSemOnce      sync.Once
	workerSem          chan struct{}
)

func kernelForInterpolation(interp Interpolation) kernelDef {
	switch interp {
	case InterpolationBilinear:
		return kernelDef{interp: InterpolationBilinear, taps: 2, kernel: linearKernel}
	case InterpolationBicubic:
		return kernelDef{interp: InterpolationBicubic, taps: 4, kernel: cubicKernel}
	case InterpolationMitchellNetravali:
		return kernelDef{interp: InterpolationMitchellNetravali, taps: 4, kernel: mitchellNetravaliKernel}
	case InterpolationLanczos2:
		return kernelDef{interp: InterpolationLanczos2, taps: 4, kernel: lanczos2Kernel}
	case InterpolationLanczos3:
		return kernelDef{interp: InterpolationLanczos3, taps: 6, kernel: lanczos3Kernel}
	default:
		return kernelDef{interp: InterpolationNearest, taps: 2, kernel: nearestKernel}
	}
}

func resizeYCbCrInterpolated(src *image.YCbCr, w, h int, interp Interpolation) *image.YCbCr {
	if interp == InterpolationNearest {
		return resizeYCbCrNearest(src, w, h)
	}
	def := kernelForInterpolation(interp)
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), src.SubsampleRatio)

	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	yPlane := resamplePlane8(src.Y, srcW, srcH, src.YStride, w, h, def)
	copyPlane8(dst.Y, dst.YStride, w, h, yPlane)

	srcCbW, srcCbH := chromaSize(src.Rect, src.SubsampleRatio)
	dstCbW, dstCbH := chromaSize(dst.Rect, dst.SubsampleRatio)
	cbPlane := resamplePlane8(src.Cb, srcCbW, srcCbH, src.CStride, dstCbW, dstCbH, def)
	crPlane := resamplePlane8(src.Cr, srcCbW, srcCbH, src.CStride, dstCbW, dstCbH, def)
	copyPlane8(dst.Cb, dst.CStride, dstCbW, dstCbH, cbPlane)
	copyPlane8(dst.Cr, dst.CStride, dstCbW, dstCbH, crPlane)

	return dst
}

func resizeGrayInterpolated(src *image.Gray, w, h int, interp Interpolation) *image.Gray {
	if interp == InterpolationNearest {
		dst := image.NewGray(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewGray(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	plane := resamplePlane8(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyPlane8(dst.Pix, dst.Stride, w, h, plane)
	return dst
}

func resizeGray16Interpolated(src *image.Gray16, w, h int, interp Interpolation) *image.Gray16 {
	if interp == InterpolationNearest {
		dst := image.NewGray16(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewGray16(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	plane := resamplePlane16(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyPlane16(dst.Pix, dst.Stride, w, h, plane)
	return dst
}

func resizeRGBAInterpolated(src *image.RGBA, w, h int, interp Interpolation) *image.RGBA {
	if interp == InterpolationNearest {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	pix := resampleRGBA8(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyRGBA8(dst.Pix, dst.Stride, w, h, pix)
	return dst
}

func resizeNRGBAInterpolated(src *image.NRGBA, w, h int, interp Interpolation) *image.NRGBA {
	if interp == InterpolationNearest {
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	pix := resampleRGBA8(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyRGBA8(dst.Pix, dst.Stride, w, h, pix)
	return dst
}

func resizeRGBA64Interpolated(src *image.RGBA64, w, h int, interp Interpolation) *image.RGBA64 {
	if interp == InterpolationNearest {
		dst := image.NewRGBA64(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewRGBA64(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	pix := resampleRGBA16(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyRGBA16(dst.Pix, dst.Stride, w, h, pix)
	return dst
}

func resizeNRGBA64Interpolated(src *image.NRGBA64, w, h int, interp Interpolation) *image.NRGBA64 {
	if interp == InterpolationNearest {
		dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
		nearestScale(dst, src)
		return dst
	}
	def := kernelForInterpolation(interp)
	dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
	srcW, srcH := src.Rect.Dx(), src.Rect.Dy()
	pix := resampleRGBA16(src.Pix, srcW, srcH, src.Stride, w, h, def)
	copyRGBA16(dst.Pix, dst.Stride, w, h, pix)
	return dst
}

func resamplePlane8(src []uint8, srcW, srcH, srcStride, dstW, dstH int, def kernelDef) []uint8 {
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)
	wx := getWeights(srcW, dstW, def, scaleX)
	wy := getWeights(srcH, dstH, def, scaleY)

	temp := getFloat32(dstW * srcH)
	parallelFor(srcH, func(start, end int) {
		for y := start; y < end; y++ {
			row := src[y*srcStride:]
			outRow := temp[y*dstW:]
			for x := 0; x < dstW; x++ {
				s := wx.start[x]
				base := x * wx.filterLength
				var sum float32
				for i := 0; i < wx.filterLength; i++ {
					xi := s + i
					if xi < 0 {
						xi = 0
					} else if xi >= srcW {
						xi = srcW - 1
					}
					sum += float32(row[xi]) * wx.coeffs[base+i]
				}
				outRow[x] = sum
			}
		}
	})

	out := make([]uint8, dstW*dstH)
	parallelFor(dstH, func(start, end int) {
		for y := start; y < end; y++ {
			s := wy.start[y]
			base := y * wy.filterLength
			row := out[y*dstW:]
			for x := 0; x < dstW; x++ {
				var sum float32
				for i := 0; i < wy.filterLength; i++ {
					yi := s + i
					if yi < 0 {
						yi = 0
					} else if yi >= srcH {
						yi = srcH - 1
					}
					sum += temp[yi*dstW+x] * wy.coeffs[base+i]
				}
				row[x] = clampToByte(sum)
			}
		}
	})

	putFloat32(temp)
	return out
}

func resamplePlane16(src []uint8, srcW, srcH, srcStride, dstW, dstH int, def kernelDef) []uint16 {
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)
	wx := getWeights(srcW, dstW, def, scaleX)
	wy := getWeights(srcH, dstH, def, scaleY)

	temp := getFloat32(dstW * srcH)
	parallelFor(srcH, func(start, end int) {
		for y := start; y < end; y++ {
			row := src[y*srcStride:]
			outRow := temp[y*dstW:]
			for x := 0; x < dstW; x++ {
				s := wx.start[x]
				base := x * wx.filterLength
				var sum float32
				for i := 0; i < wx.filterLength; i++ {
					xi := s + i
					if xi < 0 {
						xi = 0
					} else if xi >= srcW {
						xi = srcW - 1
					}
					off := xi * 2
					val := uint16(row[off])<<8 | uint16(row[off+1])
					sum += float32(val) * wx.coeffs[base+i]
				}
				outRow[x] = sum
			}
		}
	})

	out := make([]uint16, dstW*dstH)
	parallelFor(dstH, func(start, end int) {
		for y := start; y < end; y++ {
			s := wy.start[y]
			base := y * wy.filterLength
			row := out[y*dstW:]
			for x := 0; x < dstW; x++ {
				var sum float32
				for i := 0; i < wy.filterLength; i++ {
					yi := s + i
					if yi < 0 {
						yi = 0
					} else if yi >= srcH {
						yi = srcH - 1
					}
					sum += temp[yi*dstW+x] * wy.coeffs[base+i]
				}
				row[x] = clampToUint16(sum)
			}
		}
	})

	putFloat32(temp)
	return out
}

func resampleRGBA8(src []uint8, srcW, srcH, srcStride, dstW, dstH int, def kernelDef) []uint8 {
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)
	wx := getWeights(srcW, dstW, def, scaleX)
	wy := getWeights(srcH, dstH, def, scaleY)

	temp := getFloat32(dstW * srcH * 4)
	parallelFor(srcH, func(start, end int) {
		for y := start; y < end; y++ {
			row := src[y*srcStride:]
			outRow := temp[y*dstW*4:]
			for x := 0; x < dstW; x++ {
				s := wx.start[x]
				base := x * wx.filterLength
				var r, g, b, a float32
				for i := 0; i < wx.filterLength; i++ {
					xi := s + i
					if xi < 0 {
						xi = 0
					} else if xi >= srcW {
						xi = srcW - 1
					}
					off := xi * 4
					w := wx.coeffs[base+i]
					r += float32(row[off+0]) * w
					g += float32(row[off+1]) * w
					b += float32(row[off+2]) * w
					a += float32(row[off+3]) * w
				}
				outOff := x * 4
				outRow[outOff+0] = r
				outRow[outOff+1] = g
				outRow[outOff+2] = b
				outRow[outOff+3] = a
			}
		}
	})

	out := make([]uint8, dstW*dstH*4)
	parallelFor(dstH, func(start, end int) {
		for y := start; y < end; y++ {
			s := wy.start[y]
			base := y * wy.filterLength
			row := out[y*dstW*4:]
			for x := 0; x < dstW; x++ {
				var r, g, b, a float32
				for i := 0; i < wy.filterLength; i++ {
					yi := s + i
					if yi < 0 {
						yi = 0
					} else if yi >= srcH {
						yi = srcH - 1
					}
					off := (yi*dstW + x) * 4
					w := wy.coeffs[base+i]
					r += temp[off+0] * w
					g += temp[off+1] * w
					b += temp[off+2] * w
					a += temp[off+3] * w
				}
				outOff := x * 4
				row[outOff+0] = clampToByte(r)
				row[outOff+1] = clampToByte(g)
				row[outOff+2] = clampToByte(b)
				row[outOff+3] = clampToByte(a)
			}
		}
	})

	putFloat32(temp)
	return out
}

func resampleRGBA16(src []uint8, srcW, srcH, srcStride, dstW, dstH int, def kernelDef) []uint16 {
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)
	wx := getWeights(srcW, dstW, def, scaleX)
	wy := getWeights(srcH, dstH, def, scaleY)

	temp := getFloat32(dstW * srcH * 4)
	parallelFor(srcH, func(start, end int) {
		for y := start; y < end; y++ {
			row := src[y*srcStride:]
			outRow := temp[y*dstW*4:]
			for x := 0; x < dstW; x++ {
				s := wx.start[x]
				base := x * wx.filterLength
				var r, g, b, a float32
				for i := 0; i < wx.filterLength; i++ {
					xi := s + i
					if xi < 0 {
						xi = 0
					} else if xi >= srcW {
						xi = srcW - 1
					}
					off := xi * 8
					w := wx.coeffs[base+i]
					r += float32(uint16(row[off+0])<<8|uint16(row[off+1])) * w
					g += float32(uint16(row[off+2])<<8|uint16(row[off+3])) * w
					b += float32(uint16(row[off+4])<<8|uint16(row[off+5])) * w
					a += float32(uint16(row[off+6])<<8|uint16(row[off+7])) * w
				}
				outOff := x * 4
				outRow[outOff+0] = r
				outRow[outOff+1] = g
				outRow[outOff+2] = b
				outRow[outOff+3] = a
			}
		}
	})

	out := make([]uint16, dstW*dstH*4)
	parallelFor(dstH, func(start, end int) {
		for y := start; y < end; y++ {
			s := wy.start[y]
			base := y * wy.filterLength
			row := out[y*dstW*4:]
			for x := 0; x < dstW; x++ {
				var r, g, b, a float32
				for i := 0; i < wy.filterLength; i++ {
					yi := s + i
					if yi < 0 {
						yi = 0
					} else if yi >= srcH {
						yi = srcH - 1
					}
					off := (yi*dstW + x) * 4
					w := wy.coeffs[base+i]
					r += temp[off+0] * w
					g += temp[off+1] * w
					b += temp[off+2] * w
					a += temp[off+3] * w
				}
				outOff := x * 4
				row[outOff+0] = clampToUint16(r)
				row[outOff+1] = clampToUint16(g)
				row[outOff+2] = clampToUint16(b)
				row[outOff+3] = clampToUint16(a)
			}
		}
	})

	putFloat32(temp)
	return out
}

func getWeights(src, dst int, def kernelDef, scale float64) resampleWeights {
	if src <= 0 || dst <= 0 {
		return resampleWeights{}
	}
	key := weightsKey{src: src, dst: dst, interp: def.interp}
	if cached, ok := weightsCache.Load(key); ok {
		return cached.(resampleWeights)
	}
	filterLength := def.taps * int(math.Max(math.Ceil(scale), 1))
	filterFactor := math.Min(1.0/scale, 1.0)
	coeffs := make([]float32, dst*filterLength)
	start := make([]int, dst)
	for y := 0; y < dst; y++ {
		interpX := scale*(float64(y)+0.5) - 0.5
		start[y] = int(interpX) - filterLength/2 + 1
		interpX -= float64(start[y])
		base := y * filterLength
		var sum float64
		for i := 0; i < filterLength; i++ {
			in := (interpX - float64(i)) * filterFactor
			w := def.kernel(in)
			coeffs[base+i] = float32(w)
			sum += w
		}
		if sum != 0 {
			inv := float32(1.0 / sum)
			for i := 0; i < filterLength; i++ {
				coeffs[base+i] *= inv
			}
		}
	}
	weights := resampleWeights{coeffs: coeffs, start: start, filterLength: filterLength}
	weightsCache.Store(key, weights)
	return weights
}

func parallelFor(total int, fn func(start, end int)) {
	if total <= 0 {
		return
	}
	capacity := runtime.GOMAXPROCS(0)
	if maxParallelWorkers > 0 && capacity > maxParallelWorkers {
		capacity = maxParallelWorkers
	}
	if capacity < 1 {
		capacity = 1
	}
	workerSemOnce.Do(func() {
		workerSem = make(chan struct{}, capacity)
	})
	if cap(workerSem) < capacity {
		capacity = cap(workerSem)
		if capacity < 1 {
			capacity = 1
		}
	}
	workers := capacity
	if maxParallelWorkers > 0 && workers > maxParallelWorkers {
		workers = maxParallelWorkers
	}
	if workers > total {
		workers = total
	}
	if workers <= 1 {
		fn(0, total)
		return
	}
	step := (total + workers - 1) / workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		start := i * step
		end := start + step
		if end > total {
			end = total
		}
		if start >= end {
			break
		}
		workerSem <- struct{}{}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			defer func() { <-workerSem }()
			fn(s, e)
		}(start, end)
	}
	wg.Wait()
}

func getFloat32(n int) []float32 {
	bufPtr := float32Pool.Get().(*[]float32)
	buf := *bufPtr
	if cap(buf) < n {
		return make([]float32, n)
	}
	return buf[:n]
}

func putFloat32(buf []float32) {
	if buf == nil {
		return
	}
	for i := range buf {
		buf[i] = 0
	}
	buf = buf[:0]
	float32Pool.Put(&buf)
}

func nearestKernel(in float64) float64 {
	if in >= -0.5 && in < 0.5 {
		return 1
	}
	return 0
}

func linearKernel(in float64) float64 {
	in = math.Abs(in)
	if in <= 1 {
		return 1 - in
	}
	return 0
}

func cubicKernel(in float64) float64 {
	in = math.Abs(in)
	if in <= 1 {
		return in*in*(1.5*in-2.5) + 1.0
	}
	if in <= 2 {
		return in*(in*(2.5-0.5*in)-4.0) + 2.0
	}
	return 0
}

func mitchellNetravaliKernel(in float64) float64 {
	in = math.Abs(in)
	if in <= 1 {
		return (7.0*in*in*in - 12.0*in*in + 5.33333333333) * 0.16666666666
	}
	if in <= 2 {
		return (-2.33333333333*in*in*in + 12.0*in*in - 20.0*in + 10.6666666667) * 0.16666666666
	}
	return 0
}

func sinc(x float64) float64 {
	x = math.Abs(x) * math.Pi
	if x >= 1.220703e-4 {
		return math.Sin(x) / x
	}
	return 1
}

func lanczos2Kernel(in float64) float64 {
	if in > -2 && in < 2 {
		return sinc(in) * sinc(in*0.5)
	}
	return 0
}

func lanczos3Kernel(in float64) float64 {
	if in > -3 && in < 3 {
		return sinc(in) * sinc(in*0.3333333333333333)
	}
	return 0
}

func clampToByte(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func clampToUint16(v float32) uint16 {
	if v <= 0 {
		return 0
	}
	if v >= 65535 {
		return 65535
	}
	return uint16(v + 0.5)
}

func copyPlane8(dst []uint8, dstStride, dstW, dstH int, src []uint8) {
	for y := 0; y < dstH; y++ {
		copy(dst[y*dstStride:y*dstStride+dstW], src[y*dstW:(y+1)*dstW])
	}
}

func copyPlane16(dst []uint8, dstStride, dstW, dstH int, src []uint16) {
	for y := 0; y < dstH; y++ {
		row := dst[y*dstStride:]
		for x := 0; x < dstW; x++ {
			v := src[y*dstW+x]
			off := x * 2
			row[off] = uint8(v >> 8)
			row[off+1] = uint8(v)
		}
	}
}

func copyRGBA8(dst []uint8, dstStride, dstW, dstH int, src []uint8) {
	rowSize := dstW * 4
	for y := 0; y < dstH; y++ {
		copy(dst[y*dstStride:y*dstStride+rowSize], src[y*rowSize:(y+1)*rowSize])
	}
}

func copyRGBA16(dst []uint8, dstStride, dstW, dstH int, src []uint16) {
	rowSize := dstW * 4
	for y := 0; y < dstH; y++ {
		row := dst[y*dstStride:]
		for x := 0; x < rowSize; x++ {
			v := src[y*rowSize+x]
			off := x * 2
			row[off] = uint8(v >> 8)
			row[off+1] = uint8(v)
		}
	}
}
