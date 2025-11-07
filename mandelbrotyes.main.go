// Mandelbrot Interactive Viewer in Go using Ebiten
// Author: Juan Arce & ChatGPT (Senior Software Engineer & Physicist)
// Features: Mouse wheel zoom (to cursor), click & drag panning, smooth coloring, efficient rendering.

package main

import (
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	screenWidth  = 800
	screenHeight = 800
	maxIt        = 256
)

// Smooth color mapping based on normalized iteration count
func color(it int, z complex128) (r, g, b byte) {
	if it == maxIt {
		return 0x00, 0x00, 0x00
	}
	magZ := real(z)*real(z) + imag(z)*imag(z)
	if magZ == 0 {
		return 0, 0, 0
	}
	logMagZ := math.Log(magZ)
	v := float64(it) + 1.0 - math.Log(logMagZ/2)/math.Log(2.0)
	r = byte(math.Sin(0.1*v+0.0)*127 + 128)
	g = byte(math.Sin(0.1*v+2.0)*127 + 128)
	b = byte(math.Sin(0.1*v+4.0)*127 + 128)
	return
}

type Game struct {
	offscreen    *ebiten.Image
	offscreenPix []byte
	centerX      float64
	centerY      float64
	size         float64
	needsRedraw  bool

	// Mouse interaction
	prevMouseX float64
	prevMouseY float64
	dragging   bool
}

func NewGame() *Game {
	return &Game{
		offscreen:    ebiten.NewImage(screenWidth, screenHeight),
		offscreenPix: make([]byte, screenWidth*screenHeight*4),
		centerX:      -0.75,
		centerY:      0.0,
		size:         3.0,
		needsRedraw:  true,
	}
}

func (gm *Game) updateOffscreen() {
	for j := 0; j < screenHeight; j++ {
		for i := 0; i < screenWidth; i++ {
			x := (float64(i)/screenWidth-0.5)*gm.size + gm.centerX
			y := (0.5-float64(j)/screenHeight)*gm.size + gm.centerY
			c := complex(x, y)

			z := complex(0, 0)
			it := 0
			for ; it < maxIt; it++ {
				z = z*z + c
				if real(z)*real(z)+imag(z)*imag(z) > 4 {
					break
				}
			}
			r, g, b := color(it, z)
			p := 4 * (i + j*screenWidth)
			gm.offscreenPix[p+0] = r
			gm.offscreenPix[p+1] = g
			gm.offscreenPix[p+2] = b
			gm.offscreenPix[p+3] = 0xFF
		}
	}
	gm.offscreen.WritePixels(gm.offscreenPix)
}

func (g *Game) Update() error {
	// Handle zoom (mouse wheel)
	_, scrollY := ebiten.Wheel()
	if scrollY != 0 {
		mx, my := ebiten.CursorPosition()

		// Convert mouse position to complex plane coordinates
		mouseX := (float64(mx)/screenWidth-0.5)*g.size + g.centerX
		mouseY := (0.5-float64(my)/screenHeight)*g.size + g.centerY

		zoomFactor := math.Pow(1.1, -scrollY) // smooth zoom
		g.size *= zoomFactor

		// Zoom towards cursor (keep mouse position fixed in view)
		g.centerX = mouseX + (g.centerX-mouseX)*zoomFactor
		g.centerY = mouseY + (g.centerY-mouseY)*zoomFactor

		g.needsRedraw = true
	}

	// Handle panning (left mouse drag)
	mx, my := ebiten.CursorPosition()
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !g.dragging {
			g.dragging = true
			g.prevMouseX, g.prevMouseY = float64(mx), float64(my)
		} else {
			dx := float64(mx) - g.prevMouseX
			dy := float64(my) - g.prevMouseY
			g.prevMouseX, g.prevMouseY = float64(mx), float64(my)

			// Translate movement into Mandelbrot coordinates
			g.centerX -= dx / screenWidth * g.size
			g.centerY += dy / screenHeight * g.size
			g.needsRedraw = true
		}
	} else {
		g.dragging = false
	}

	// Reset view
	if ebiten.IsKeyPressed(ebiten.KeyR) {
		g.centerX = -0.75
		g.centerY = 0.0
		g.size = 3.0
		g.needsRedraw = true
	}

	if g.needsRedraw {
		g.updateOffscreen()
		g.needsRedraw = false
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.DrawImage(g.offscreen, nil)
	ebiten.SetWindowTitle(
		"Mandelbrot Explorer | Zoom: Mouse Wheel | Pan: Drag Left Mouse | Reset: R",
	)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Mandelbrot Explorer (Go + Ebiten)")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
