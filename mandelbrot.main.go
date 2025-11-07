// Copyright 2017 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	screenWidth  = 640
	screenHeight = 640
	maxIt        = 256 // Increased iterations for better detail when zooming
)

// --- Color Function: Smooth Julia Set-like Coloring ---

// color calculates a smooth color based on the escape time 'it' and final complex value 'z'.
func color(it int, z complex128) (r, g, b byte) {
	if it == maxIt {
		// Points in the set are black
		return 0x00, 0x00, 0x00
	}

	// Calculate Normalized Iteration Count (smooth coloring)
	// v = it + 1 - log(log(|z|)) / log(2)
	magZ := real(z)*real(z) + imag(z)*imag(z) // Using |z|^2 as it avoids a sqrt, and log(sqrt(x)) = 0.5 * log(x)
	
	// A small check to avoid log(0) which happens if magZ is very close to zero
	if magZ == 0 {
		return 0x00, 0x00, 0x00
	}
	
	// Since the bailout is 4, log(4) = 2. The formula uses log(2) in the denominator, 
	// but since we are interested in the fractional part, we can simplify the formula 
	// slightly and map the result to a color gradient.
	
	// We use the log of the magnitude squared.
	// We'll use a simple, aesthetically pleasing sine wave color map.
	logMagZ := math.Log(magZ)
	v := float64(it) + 1.0 - math.Log(logMagZ/2) / math.Log(2.0)
	
	// Map the fractional iteration count 'v' to an HSL or sine-based RGB color.
	// Adjust these constants for a different palette.
	r = byte(math.Sin(0.1*v+0.0)*127 + 128)
	g = byte(math.Sin(0.1*v+2.0)*127 + 128)
	b = byte(math.Sin(0.1*v+4.0)*127 + 128)

	return r, g, b
}

// --- Game Structure and Methods ---

type Game struct {
	offscreen    *ebiten.Image
	offscreenPix []byte
	centerX      float64
	centerY      float64
	size         float64 // Width of the view in the complex plane
	needsRedraw  bool
}

func NewGame() *Game {
	g := &Game{
		offscreen:    ebiten.NewImage(screenWidth, screenHeight),
		offscreenPix: make([]byte, screenWidth*screenHeight*4),
		// Initial View: the whole Mandelbrot set
		centerX: -0.75, 
		centerY: 0.0,
		size:    3.0,
		needsRedraw: true,
	}
	// Initial image will be drawn in the first Update call
	return g
}

func (gm *Game) updateOffscreen(centerX, centerY, size float64) {
	// The complex plane width/height is 'size'.
	// This is the Mandelbrot Set calculation (escape time algorithm).
	for j := 0; j < screenHeight; j++ {
		for i := 0; i < screenWidth; i++ {
			// Map pixel (i, j) to complex coordinate c = x + yi
			x := float64(i)*size/screenWidth - size/2 + centerX
			y := (screenHeight-float64(j))*size/screenHeight - size/2 + centerY
			c := complex(x, y)
			
			z := complex(0, 0)
			it := 0
			
			// Max Iterations loop
			for ; it < maxIt; it++ {
				z = z*z + c
				// Check for bailout condition: |z|^2 > 4.0
				if real(z)*real(z)+imag(z)*imag(z) > 4.0 {
					break
				}
			}
			
			// Get color using the smooth coloring function
			r, g, b := color(it, z)
			
			// Write the color to the pixel buffer
			p := 4 * (i + j*screenWidth)
			gm.offscreenPix[p] = r
			gm.offscreenPix[p+1] = g
			gm.offscreenPix[p+2] = b
			gm.offscreenPix[p+3] = 0xff // Alpha
		}
	}
	// Update the Ebiten image from the pixel buffer
	gm.offscreen.WritePixels(gm.offscreenPix)
}

func (g *Game) Update() error {
	const (
		panSpeed   = 0.05 // Pan distance relative to current view size
		zoomFactor = 1.1  // Zoom step (10% change)
	)

	// --- Input Handling for Pan and Zoom ---
	
	// Panning (Navigation)
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.centerX -= g.size * panSpeed
		g.needsRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.centerX += g.size * panSpeed
		g.needsRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.centerY += g.size * panSpeed
		g.needsRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.centerY -= g.size * panSpeed
		g.needsRedraw = true
	}

	// Zooming
	if ebiten.IsKeyPressed(ebiten.KeyI) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.size /= zoomFactor
		g.needsRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyO) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		g.size *= zoomFactor
		g.needsRedraw = true
	}
	
	// Reset to initial view (Optional feature)
	if ebiten.IsKeyPressed(ebiten.KeyR) {
		g.centerX = -0.75
		g.centerY = 0.0
		g.size = 3.0
		g.needsRedraw = true
	}

	// Only recalculate the fractal if the view has changed
	if g.needsRedraw {
		g.updateOffscreen(g.centerX, g.centerY, g.size)
		g.needsRedraw = false
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw the pre-calculated offscreen image to the main screen
	screen.DrawImage(g.offscreen, nil)
	
	// Optional: Display controls
	ebiten.SetWindowTitle("Mandelbrot (Ebitengine Demo) - Pan: Arrows | Zoom: I/O or Mouse Clicks | Reset: R")
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Mandelbrot (Ebitengine Demo)")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
