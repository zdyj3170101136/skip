// go语言运行shell命令
package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error, args ...interface{}) {
	if err != nil {
		if len(args) > 0 {
			doing = fmt.Sprintf(doing, args...)
		}
		out("error %s: %s\n", doing, err)
		os.Exit(1)
	}
}

const (
	imgPathInPhone = "/sdcard/screenshot.png"
	imgDirName     = "imgs"
)

const (
	SleepLess         = 1000 // sleep at least, wait for skip done
	SleepRandom       = 1000 // random sleep, prevent cheat
	PressRandomFactor = 2    // random press, prevent cheat
	MillSecond        = 1000 * 1000
)

const (
	JumpRatio = 1.363
)

func main() {
	path, _ := filepath.Abs(os.Args[0])
	imgDirPath := filepath.Join(filepath.Dir(path), imgDirName)
	if _, err := os.Stat(imgDirPath); !os.IsNotExist(err) {
		checkErr("Init path", errors.New(""), fmt.Sprintf("Old img Path exist:%v", imgDirPath))
	}
	os.MkdirAll(imgDirPath, 0755)

	configImgPathChan := make(chan string)
	configResChan := make(chan config)
	go getConfig(configImgPathChan, configResChan)

	posImgPathChan := make(chan string)
	posResChan := make(chan pos)
	go findPos(posImgPathChan, posResChan)

	nextCenterImgPathChan := make(chan string)
	nextCenterResChan := make(chan pos)
	go findNextCenter(nextCenterImgPathChan, nextCenterResChan)

	var lastpos pos
	var lastCenter pos
	var jumpRatio float64
	jumpRatio = JumpRatio
	i := 0

	for {
		err := exec.Command("adb", "shell", "/system/bin/screencap", "-p", imgPathInPhone).Run()
		checkErr("ScreenshotStart", err)

		imgPath := filepath.Join(imgDirPath, strconv.Itoa(i)+".png")
		err = exec.Command("adb", "pull", imgPathInPhone, imgPath).Run()
		checkErr("PullPhoto", err, imgPath)

		posImgPathChan <- imgPath
		nextCenterImgPathChan <- imgPath
		configImgPathChan <- imgPath

		config := <-configResChan
		log.Printf("Photo Size:%v", config)

		pos := <-posResChan
		log.Printf("Find Chess Position:X: %v Y: %v", pos.X, pos.Y)

		nextCenter := <-nextCenterResChan
		log.Printf("Find Next Center:X: %v Y: %v", nextCenter.X, nextCenter.Y)

		jumpRatio = adjust(jumpRatio, lastpos, lastCenter, pos)
		fmt.Println(jumpRatio)

		pixelDistance := math.Sqrt(math.Abs(math.Pow(float64(nextCenter.X-pos.X), 2)) + math.Abs(math.Pow(float64(nextCenter.Y-pos.Y), 2)))
		pressTime := jumpRatio * pixelDistance

		// random swipe, prevent cheat
		pressX := rand.Intn(config.weight / PressRandomFactor)
		pressY := rand.Intn(config.height / PressRandomFactor)

		err = exec.Command("adb", "shell", fmt.Sprintf("input swipe %d %d %d %d %d", pressX, pressY, pressX, pressY, int(pressTime))).Run()
		checkErr("Skip", err, imgPath)

		time.Sleep(time.Duration(MillSecond * (SleepLess + rand.Intn(SleepRandom))))
		log.Printf("swipe %d %d: %d", pressX, pressY, int(pressTime))

		lastpos = pos
		lastCenter = nextCenter

		i++
		if i > 5000 {
			break
		}
	}
}

func adjust(jumpRatio float64, lastpos, lastCenter, nextpos pos) float64 {
	if lastpos.X == 0 {
		return jumpRatio
	}

	return jumpRatio
	expect := math.Sqrt(math.Abs(math.Pow(float64(lastCenter.X-lastpos.X), 2)) + math.Abs(math.Pow(float64(lastCenter.Y-lastpos.Y), 2)))
	really := math.Sqrt(math.Abs(math.Pow(float64(nextpos.X-lastpos.X), 2)) + math.Abs(math.Pow(float64(nextpos.Y-lastpos.Y), 2)))

	ratio := really / (expect * jumpRatio)
	expectJumpRatio := 1 / ratio

	fmt.Println(expect, really, expectJumpRatio, jumpRatio)

	// slowly
	jumpRatio += (expectJumpRatio - jumpRatio) / 3
	return jumpRatio
}

const (
	OffsetOfYToFindTop    = 500 // avoid touching scores
	OffsetOfXToFindTop    = 10
	OffsetOfXToFindCenter = 400 // find center in range[topX - OffsetOfXToFindCenter, topX + OffsetOfXToFindCenter]
	OffsetOfYToFindCenter = 300 // find center in range[topY, topY + OffsetOfYToFindCenter]
	BlockOutOfRange       = 50  // we think either left or right point is out of window
)

type rgb struct {
	r uint32
	g uint32
	b uint32
}

const (
	sampleTopOffset    = 200 // sample point offset
	sampleBottomOffSet = 500
)

type topbottom struct {
	top    rgb
	bottom rgb
}
type colorType map[rgb]topbottom

var colorModel = colorType{}

func getsample(p image.Image) (rgb, rgb) {
	colorToOccurence := make(map[rgb]int)
	var topSample rgb
	var bottomSample rgb

	// gradient background
	// Upper corner target color
	topSample.r, topSample.g, topSample.b = At(p, 0, sampleTopOffset)

	if v, isfound := colorModel[topSample]; isfound {
		return v.top, v.bottom
	}

	log.Println("Wrong:unknown color")

	var color rgb
	for i := 0; i < p.Bounds().Max.X; i++ {
		color.r, color.g, color.b = At(p, i, p.Bounds().Max.Y-sampleBottomOffSet)
		colorToOccurence[color]++
	}
	log.Printf("NewColor: %v", topSample)

	maxOccurence := 0
	for k, v := range colorToOccurence {
		if v > maxOccurence {
			maxOccurence = v
			bottomSample = k
		}
	}

	colorModel[topSample] = topbottom{
		top:    topSample,
		bottom: bottomSample,
	}

	return topSample, bottomSample
}

const (
	chessLength    = 210
	chessWidth     = 100
	tooCloseOffset = 50 // we use to judge if they are too close or just out of windows
)

func findNextCenter(imgPathChan chan string, resChan chan pos) {
	defer func() {
		resChan <- pos{}
	}()

	for imgPath := range imgPathChan {
		f, err := os.OpenFile(imgPath, os.O_RDONLY, 0644)
		if err != nil {
			return
		}

		p, err := png.Decode(f)
		if err != nil {
			return
		}

		// get sample
		topSample, bottomSample := getsample(p)

		utr, utg, utb := topSample.r, topSample.g, topSample.b
		ltr, ltg, ltb := bottomSample.r, bottomSample.g, bottomSample.b

		minr := uint32(math.Min(float64(utr), float64(ltr)))
		maxr := uint32(math.Max(float64(utr), float64(ltr)))

		ming := uint32(math.Min(float64(utg), float64(ltg)))
		maxg := uint32(math.Max(float64(utg), float64(ltg)))

		minb := uint32(math.Min(float64(utb), float64(ltb)))
		maxb := uint32(math.Max(float64(utb), float64(ltb)))

		topX, topY := 0, 0

		// wait for find position done
		chessCoordinate := <-syncChan

	loop:
		for y := p.Bounds().Min.Y + OffsetOfYToFindTop; y <= p.Bounds().Max.Y; y++ {
			for x := p.Bounds().Min.X + OffsetOfXToFindTop; x < p.Bounds().Max.X-OffsetOfXToFindTop; x++ {
				r, g, b := At(p, x, y)

				if !matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb) &&
					math.Sqrt(math.Abs(math.Pow(float64(chessCoordinate.X-x), 2)+math.Abs(math.Pow(float64(chessCoordinate.Y-y), 2)))) > chessLength {
					topX = x
					topY = y
					break loop
				}
			}
		}

		leftX, rightX, leftY, rightY := p.Bounds().Max.X, 0, 0, 0

		ntr, ntg, ntb := At(p, topX, topY)

		// search right
		for y := topY; y <= topY+OffsetOfYToFindCenter; y++ {
			for x := topX; x <= topX+OffsetOfXToFindCenter; x++ {
				r, g, b := At(p, x, y)

				if matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb) {
					break
				}

				if match(r, g, b, ntr, ntg, ntb, deviation) {
					if x > rightX {
						rightX = x
						rightY = y
					}
				}
			}
		}

		// search left
		for y := topY; y <= topY+OffsetOfYToFindCenter; y++ {
			for x := topX; x >= topX-OffsetOfXToFindCenter; x-- {
				r, g, b := At(p, x, y)

				if matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb) {
					break
				}

				if match(r, g, b, ntr, ntg, ntb, deviation) {
					if x < leftX {
						leftX = x
						leftY = y
					}
				}
			}
		}

		centerX := (leftX + rightX) / 2
		centerY := (leftY + rightY) / 2

		// case: block out of range or block too close
		if math.Abs(float64(topX-leftX)-float64(rightX-topX)) > BlockOutOfRange {
			if leftX-p.Bounds().Min.X < tooCloseOffset || p.Bounds().Max.X-rightX < tooCloseOffset {
				// out of range
				if topX-leftX > rightX-topX {
					// right point out of range
					centerX = topX
					centerY = leftY

				} else {
					// left point out of range
					centerX = topX
					centerY = rightY
				}
			} else {
				// too close
				if topX-leftX > rightX-topX {
					// chess in left
					centerX = topX
					centerY = rightY

				} else {
					// chess in right
					centerX = topX
					centerY = leftX
				}
			}
		}

		// open again
		f, err = os.OpenFile(imgPath, os.O_RDONLY, 0644)
		if err != nil {
			return
		}

		p, err = png.Decode(f)
		if err != nil {
			return
		}

		drawRectangle(imgPath, p, topX, topY, leftX, leftY, rightX, rightY, centerX, centerY)

		f.Close()

		resChan <- pos{
			X: centerX,
			Y: centerY,
		}
	}
}

type config struct {
	weight int
	height int
}

func getConfig(imgPathChan chan string, resChan chan config) {
	defer func() {
		resChan <- config{}
	}()

	for imgPath := range imgPathChan {
		f, err := os.OpenFile(imgPath, os.O_RDONLY, 0644)
		if err != nil {
			return
		}

		c, err := png.DecodeConfig(f)
		if err != nil {
			return
		}

		f.Close()

		resChan <- config{
			weight: c.Width,
			height: c.Height,
		}
	}
}

const (
	RChess        = 40
	GChess        = 43
	BChess        = 86
	RectangleSize = 10
)

type pos struct {
	X int
	Y int
}

var syncChan = make(chan pos, 0)

func findPos(imgPathChan chan string, resChan chan pos) {
	defer func() {
		resChan <- pos{}
	}()

	for imgPath := range imgPathChan {
		f, err := os.OpenFile(imgPath, os.O_RDONLY, 0644)
		if err != nil {
			return
		}

		p, err := png.Decode(f)
		if err != nil {
			return
		}

		minX, maxX, maxY := p.Bounds().Max.X, 0, 0
		beginFindX, endFindX, beginFindY, endFindY := p.Bounds().Min.X, p.Bounds().Max.X, p.Bounds().Min.Y, p.Bounds().Max.Y
		hadTouchChess := false

		for y := beginFindY; y <= endFindY; y++ {
			for x := beginFindX; x <= endFindX; x++ {
				r, g, b := At(p, x, y)

				if match(r, g, b, RChess, GChess, BChess, deviation) {
					if !hadTouchChess {
						beginFindX = x - (chessWidth / 2)
						endFindX = x + (chessWidth / 2)
						beginFindY = y
						endFindY = y + chessLength
						hadTouchChess = true
					}

					minX = int(math.Min(float64(x), float64(minX)))
					maxX = int(math.Max(float64(x), float64(maxX)))
					maxY = int(math.Max(float64(y), float64(maxY)))
				}
			}
		}

		chessCoordinate := pos{
			X: (minX + maxX) / 2,
			Y: maxY,
		}

		drawRectangle(imgPath, p, chessCoordinate.X, chessCoordinate.Y)

		// notify find center can begin to find center
		syncChan <- chessCoordinate

		f.Close()

		resChan <- chessCoordinate
	}
}

func At(p image.Image, x int, y int) (uint32, uint32, uint32) {
	r, g, b, _ := p.At(x, y).RGBA()
	r = r >> 8
	g = g >> 8
	b = b >> 8
	return r, g, b
}

const (
	deviation = 16
)

func match(r, g, b, tr, tg, tb, t uint32) bool {
	if r > tr-t && r < tr+t && g > tg-t && g < tg+t && b > tb-t && b < tb+t {
		return true
	}

	return false
}

func matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb uint32) bool {
	if r >= minr && r <= maxr && g >= ming && g <= maxg && b >= minb && b <= maxb {
		return true
	}

	return false
}

func drawRectangle(imgPath string, p image.Image, pos ...int) {
	myimage := image.NewRGBA(p.Bounds())
	myred := color.RGBA{200, 0, 0, 255}

	// copy first
	draw.Draw(myimage, myimage.Bounds(), p, p.Bounds().Min, draw.Over)

	for i := 0; i < len(pos); i += 2 {
		x, y := pos[i], pos[i+1]
		red_rect := image.Rect(x-RectangleSize, y-RectangleSize, x+RectangleSize, y+RectangleSize)
		// draw rectangler
		draw.Draw(myimage, red_rect, image.NewUniform(myred), p.Bounds().Min, draw.Over)
	}

	myfile, err := os.OpenFile(imgPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}

	png.Encode(myfile, myimage)
	myfile.Sync()
	myfile.Close()
}
