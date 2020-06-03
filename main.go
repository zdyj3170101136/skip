// go语言运行shell命令
package main

import (
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
	imgPathInPhone    = "/sdcard/screenshot.png"
	imgDirName        = "imgs"
	myPositionDirName = "myPos"
	nextCenterDirName = "nextCenter"
)

const (
	SleepLess   = 200  // sleep at least, wait for skip done
	SleepRandom = 1000 // random sleep, prevent cheat
	MillSecond  = 1000 * 1000
)

const (
	JumpRatio = 1.39
)

func main() {
	path, _ := filepath.Abs(os.Args[0])
	imgDirPath := filepath.Join(filepath.Dir(path), imgDirName)
	if _, err := os.Stat(imgDirPath); err == nil {
		os.Remove(imgDirPath)
	}
	os.MkdirAll(imgDirPath, 0755)
	os.MkdirAll(filepath.Join(imgDirPath, myPositionDirName), 0755)
	os.MkdirAll(filepath.Join(imgDirPath, nextCenterDirName), 0755)

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
		pressX := rand.Intn(config.weight / 2)
		pressY := rand.Intn(config.height / 2)

		err = exec.Command("adb", "shell", fmt.Sprintf("input swipe %d %d %d %d %d", pressX, pressY, pressX, pressY, int(pressTime))).Run()
		checkErr("Skip", err, imgPath)

		time.Sleep(time.Duration(MillSecond * (SleepLess + rand.Intn(SleepRandom))))
		log.Printf("swipe %d %d: %d", pressX, pressY, int(pressTime))

		lastpos = pos
		lastCenter = nextCenter

		i++
		if i > 2 {
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

		// 渐变色背景，多点采样
		// 左下角
		bltr, bltg, bltb := At(p, 200, 100)
		// 右下角
		brtr, brtg, brtb := At(p, 900, 2000)

		minr := uint32(math.Min(float64(bltr), float64(brtr)))
		maxr := uint32(math.Max(float64(bltr), float64(brtr)))

		ming := uint32(math.Min(float64(bltg), float64(brtg)))
		maxg := uint32(math.Max(float64(bltg), float64(brtg)))

		minb := uint32(math.Min(float64(bltb), float64(brtb)))
		maxb := uint32(math.Max(float64(bltb), float64(brtb)))

		fmt.Println(bltr, brtg, brtb)
		fmt.Println(brtr, bltg, bltb)

		topX, topY := 0, 0

	loop:
		// prevent touch button on right top
		for y := 300; y <= p.Bounds().Max.Y-200; y++ {
			// prevent touch number in left top and search range
			for x := p.Bounds().Min.X + 300; x <= 7*p.Bounds().Max.X/8; x++ {
				r, g, b := At(p, x, y)

				if !matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb, 16) {
					topX = x
					topY = y
					break loop
				}
			}
		}

		rightX, rightY := topX, topY

		ntr, ntg, ntb := At(p, topX, topY)

	loop2:
		for y := rightY + 30; y <= p.Bounds().Max.Y-200; y++ {
			for x := rightX; x <= 7*p.Bounds().Max.X/8; x++ {
				r, g, b := At(p, x, y)

				if !match(r, g, b, ntr, ntg, ntb, 16) {
					if x >= rightX {
						rightX = x
						rightY = y
						break
					} else {
						break loop2
					}

				}
			}
		}

		drawRectangle(filepath.Join(filepath.Dir(imgPath), nextCenterDirName, filepath.Base(imgPath)), p, topX, topY, rightX, rightY, topX, rightY)

		f.Close()

		resChan <- pos{
			X: topX,
			Y: rightY,
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
		for x := p.Bounds().Min.X; x <= p.Bounds().Max.X; x++ {
			for y := p.Bounds().Min.Y; y <= p.Bounds().Max.Y; y++ {
				r, g, b := At(p, x, y)

				if match(r, g, b, RChess, GChess, BChess, 16) {
					minX = int(math.Min(float64(x), float64(minX)))
					maxX = int(math.Max(float64(x), float64(maxX)))
					maxY = int(math.Max(float64(y), float64(maxY)))
				}
			}
		}

		drawRectangle(filepath.Join(filepath.Dir(imgPath), myPositionDirName, filepath.Base(imgPath)), p, (minX+maxX)/2, maxY)

		f.Close()

		resChan <- pos{
			X: (minX + maxX) / 2,
			Y: maxY,
		}
	}
}

func At(p image.Image, x int, y int) (uint32, uint32, uint32) {
	r, g, b, _ := p.At(x, y).RGBA()
	r = r >> 8
	g = g >> 8
	b = b >> 8
	return r, g, b
}

func match(r, g, b, tr, tg, tb, t uint32) bool {
	if r > tr-t && r < tr+t && g > tg-t && g < tg+t && b > tb-t && b < tb+t {
		return true
	}

	return false
}

func matchShading(r, g, b, minr, ming, minb, maxr, maxg, maxb, t uint32) bool {
	if r > minr-t && r < maxr+t && g > ming-t && g < maxg+t && b > minb-t && b < maxb+t {
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
	myfile.Close()
}
