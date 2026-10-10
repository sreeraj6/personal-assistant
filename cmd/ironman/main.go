package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

//go:embed ironman.png
var ironmanPNG []byte

const (
	winWidth  = 320
	winHeight = 160
)

var messages = []string{
	"Hey, where's your update?",
	"Jarvis says you haven't logged anything.",
	"Suit up and update your tracker!",
	"Tony Stark never forgets to log his work.",
	"Another 30 min? Time to update, hero.",
	"Even Iron Man files status reports.",
	"Your Excel misses you.",
	"Click me and tell me what you did!",
	"Villains are logging updates, are you?",
	"Proof that you're still alive — update now!",
}

type game struct {
	img      *ebiten.Image
	msg      string
	centerX  int
	centerY  int
	y        float64
	progress float64
	speed    float64
	hovering bool
	hoverT   float64
	flyingUp bool
	done     bool
	clicked  bool
}

func newGame() *game {
	img, _, err := image.Decode(bytes.NewReader(ironmanPNG))
	if err != nil {
		panic(err)
	}

	sw, sh := ebiten.ScreenSizeInFullscreen()
	g := &game{
		img:     ebiten.NewImageFromImage(img),
		msg:     messages[rand.Intn(len(messages))],
		centerX: (sw - winWidth) / 2,
		centerY: (sh - winHeight) / 2,
		y:       float64(sh),
		speed:   0.035 + rand.Float64()*0.01,
	}
	return g
}

func (g *game) Update() error {
	if g.done {
		return nil
	}

	if g.flyingUp {
		x, y := ebiten.WindowPosition()
		ebiten.SetWindowPosition(x, y-22)
		if y < -300 {
			g.done = true
			return ebiten.Termination
		}
		return nil
	}

	if !g.hovering {
		// Rise from bottom middle to center
		g.progress += g.speed
		if g.progress >= 1.0 {
			g.progress = 1.0
			g.hovering = true
		}
		targetY := float64(g.centerY)
		g.y = float64(g.screenHeight()) + (targetY-float64(g.screenHeight()))*easeOutBack(g.progress)
		ebiten.SetWindowPosition(g.centerX, int(g.y))
	} else {
		// Gentle hover in center until the user clicks.
		g.hoverT += 0.05
		offset := int(math.Sin(g.hoverT) * 8)
		ebiten.SetWindowPosition(g.centerX, g.centerY+offset)
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !g.clicked {
		g.clicked = true
		notifyMainApp()
		g.flyingUp = true
	}

	return nil
}

func (g *game) screenHeight() int {
	_, h := ebiten.ScreenSizeInFullscreen()
	return h
}

func easeOutBack(t float64) float64 {
	c1 := 1.70158
	c3 := c1 + 1
	return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
}

func (g *game) Draw(screen *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Translate(0, 50)
	screen.DrawImage(g.img, op)

	vector.DrawFilledRect(screen, 75, 10, 230, 55, color.RGBA{0, 0, 0, 180}, false)
	drawWrappedText(screen, g.msg, basicfont.Face7x13, 82, 24, 215, color.White)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return winWidth, winHeight
}

var startTime time.Time

func main() {
	rand.Seed(time.Now().UnixNano())
	startTime = time.Now()

	ebiten.SetScreenTransparent(true)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowTitle("Iron Man Reminder")

	g := newGame()
	if err := ebiten.RunGame(g); err != nil {
		fmt.Println("reminder closed:", err)
	}
}

// notifyMainApp writes a trigger file that the main metracker app watches for.
func notifyMainApp() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	trigger := filepath.Join(home, ".metracker", "add-entry.trigger")
	_ = os.WriteFile(trigger, []byte("click\n"), 0644)
}

func drawWrappedText(dst *ebiten.Image, s string, f font.Face, x, y, width int, c color.Color) {
	words := splitWords(s)
	lineHeight := f.Metrics().Height.Ceil() + 2
	line := ""
	py := y
	for _, w := range words {
		test := line + w + " "
		if line == "" {
			test = w
		}
		bw := font.MeasureString(f, test).Ceil()
		if bw > width && line != "" {
			text.Draw(dst, line, f, x, py, c)
			line = w + " "
			py += lineHeight
		} else {
			line = test
		}
	}
	if line != "" {
		text.Draw(dst, line, f, x, py, c)
	}
}

func splitWords(s string) []string {
	var out []string
	w := ""
	for _, r := range s {
		if r == ' ' {
			if w != "" {
				out = append(out, w)
				w = ""
			}
		} else {
			w += string(r)
		}
	}
	if w != "" {
		out = append(out, w)
	}
	return out
}
