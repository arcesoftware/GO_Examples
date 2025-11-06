package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/images"
)

const (
	screenWidth  = 640
	screenHeight = 480
	maxParticles = 8000 // Increased limit to stress the new batching system!
)

var smokeImage *ebiten.Image
var smokeImageW, smokeImageH float64 // Width and Height of the source image

func init() {
	// Decode an image from the image file's byte slice.
	img, _, err := image.Decode(bytes.NewReader(images.Smoke_png))
	if err != nil {
		log.Fatal(err)
	}
	smokeImage = ebiten.NewImageFromImage(img)

	// Pre-calculate image dimensions for texture coordinates
	smokeImageW = float64(smokeImage.Bounds().Dx())
	smokeImageH = float64(smokeImage.Bounds().Dy())
}

// Particle struct remains the same (CPU side logic)
type Particle struct {
	x, y            float64
	vx, vy          float64
	lifetime        int
	maxLife         int
	img             *ebiten.Image
	baseScale       float64
	angle           float64
	angularVelocity float64
	baseAlpha       float32
	color           *color.RGBA
	active          bool
}

func (p *Particle) update() {
	if !p.active {
		return
	}

	p.lifetime++
	if p.lifetime >= p.maxLife {
		p.active = false
		return
	}

	p.x += p.vx
	p.y += p.vy
	p.angle += p.angularVelocity
}

// newParticle is unchanged, initializing a particle
func newParticle(img *ebiten.Image, emitterX, emitterY float64) *Particle {
	maxLife := rand.IntN(60) + 240
	angle := rand.Float64() * math.Pi / 3.0
	if rand.IntN(2) == 0 {
		angle = -angle
	}
	angle += math.Pi / 2.0

	speed := rand.Float64()*0.4 + 0.1
	updraft := -1.0

	vx := math.Cos(angle) * speed
	vy := math.Sin(angle)*speed + updraft

	r := uint8(0xc0 + rand.IntN(0x3f))
	g := uint8(0xc0 + rand.IntN(0x3f))
	b := uint8(0xc0 + rand.IntN(0x3f))

	return &Particle{
		img: img,

		active:   true,
		maxLife:  maxLife,
		lifetime: 0,

		x:  emitterX,
		y:  emitterY,
		vx: vx,
		vy: vy,

		angle:           rand.Float64() * 2 * math.Pi,
		angularVelocity: rand.Float64() * 0.03 * (rand.Float64()*2 - 1),
		baseScale:       rand.Float64()*0.1 + 0.3,
		baseAlpha:       0.8,
		color:           &color.RGBA{R: r, G: g, B: b, A: 0xff},
	}
}

// --- Game Structure and Optimization ---

type Game struct {
	particles []*Particle
	emitterX  float64
	emitterY  float64

	// ** NEW: Pre-allocated buffers for DrawTriangles **
	// These slices are reused every frame, eliminating runtime memory allocations.
	vertices []ebiten.Vertex
	indices  []uint16
}

func (g *Game) allocateParticle() *Particle {
	for i := range g.particles {
		if !g.particles[i].active {
			return g.particles[i]
		}
	}

	if len(g.particles) < maxParticles {
		p := &Particle{}
		g.particles = append(g.particles, p)
		return p
	}
	return nil
}

func (g *Game) Update() error {
	if g.particles == nil {
		g.particles = make([]*Particle, 0, maxParticles)
		g.emitterX = screenWidth / 2
		g.emitterY = screenHeight / 2

		// Pre-allocate DrawTriangles buffers (4 vertices and 6 indices per particle)
		g.vertices = make([]ebiten.Vertex, 0, maxParticles*4)
		g.indices = make([]uint16, 0, maxParticles*6)
	}

	// Emitter and particle update logic is the same
	if len(g.particles) < maxParticles && rand.IntN(3) < 2 {
		if p := g.allocateParticle(); p != nil {
			*p = *newParticle(smokeImage, g.emitterX, g.emitterY)
		}
	}

	for _, p := range g.particles {
		if p.active {
			p.update()
		}
	}

	g.emitterX += rand.Float64()*0.5 - 0.25
	g.emitterY -= 0.1

	return nil
}

// --- The Critical Draw Function Refactor ---

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0x66, G: 0x99, B: 0xcc, A: 0xff})

	// Reset the buffers for the new frame
	g.vertices = g.vertices[:0]
	g.indices = g.indices[:0]

	activeCount := 0

	// Source image bounds for texture coordinates
	sx0, sy0 := 0.0, 0.0
	sx1, sy1 := smokeImageW, smokeImageH

	halfW, halfH := smokeImageW/2.0, smokeImageH/2.0

	for _, p := range g.particles {
		if !p.active {
			continue
		}

		activeCount++

		// Calculate dynamic properties (Scale and Alpha)
		rate := float64(p.lifetime) / float64(p.maxLife)
		scale := p.baseScale * (0.8 + 0.5*rate)

		var alpha float32
		if rate < 0.2 {
			alpha = float32(rate / 0.2)
		} else if rate > 0.8 {
			alpha = float32((1 - rate) / 0.2)
		} else {
			alpha = 1.0
		}
		alpha *= p.baseAlpha

		// Color Scale
		cr := float32(p.color.R) / 0xff * alpha
		cg := float32(p.color.G) / 0xff * alpha
		cb := float32(p.color.B) / 0xff * alpha
		ca := alpha // Alpha is already factored into the component colors via pre-multiplied alpha

		// Geometry Matrix for this particle
		var geo ebiten.GeoM
		geo.Translate(-halfW, -halfH) // 1. Move to center
		geo.Rotate(p.angle)           // 2. Rotate
		geo.Scale(scale, scale)       // 3. Scale
		geo.Translate(p.x, p.y)       // 4. Translate to final position

		// Calculate the four vertices of the quad
		vIndex := uint16(len(g.vertices))

		// 1. Top-Left
		vx, vy := geo.Apply(0, 0)
		g.vertices = append(g.vertices, ebiten.Vertex{
			DstX: float32(vx), DstY: float32(vy), SrcX: float32(sx0), SrcY: float32(sy0), ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
		})

		// 2. Bottom-Left
		vx, vy = geo.Apply(0, smokeImageH)
		g.vertices = append(g.vertices, ebiten.Vertex{
			DstX: float32(vx), DstY: float32(vy), SrcX: float32(sx0), SrcY: float32(sy1), ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
		})

		// 3. Top-Right
		vx, vy = geo.Apply(smokeImageW, 0)
		g.vertices = append(g.vertices, ebiten.Vertex{
			DstX: float32(vx), DstY: float32(vy), SrcX: float32(sx1), SrcY: float32(sy0), ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
		})

		// 4. Bottom-Right
		vx, vy = geo.Apply(smokeImageW, smokeImageH)
		g.vertices = append(g.vertices, ebiten.Vertex{
			DstX: float32(vx), DstY: float32(vy), SrcX: float32(sx1), SrcY: float32(sy1), ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
		})

		// Indices for the two triangles that form the quad (0, 1, 2) and (1, 2, 3)
		g.indices = append(g.indices,
			vIndex, vIndex+1, vIndex+2,
			vIndex+1, vIndex+3, vIndex+2,
		)
	}

	// ** Single Draw Call for ALL particles **
	// This is the core optimization for high FPS.
	if activeCount > 0 {
		op := &ebiten.DrawTrianglesOptions{
			CompositeMode: ebiten.CompositeModeLighter, // Lighter is often better for smoke/fire
		}
		screen.DrawTriangles(g.vertices, g.indices, smokeImage, op)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f\nActive Particles: %d/%d (Capacity)", ebiten.ActualTPS(), activeCount, cap(g.particles)))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("High-Performance Particles (Ebitengine Demo)")
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
