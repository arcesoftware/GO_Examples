package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	screenWidth  = 800
	screenHeight = 600
	maxParticles = 8000
	defaultTexW  = 32
	defaultTexH  = 32
)

var (
	fireImage  *ebiten.Image
	fireImageW float64
	fireImageH float64
)

func init() {
	rand.Seed(time.Now().UnixNano())

	// Procedural circular alpha texture
	img := image.NewRGBA(image.Rect(0, 0, defaultTexW, defaultTexH))
	cx, cy := defaultTexW/2.0, defaultTexH/2.0
	maxR := math.Hypot(cx, cy)
	for y := 0; y < defaultTexH; y++ {
		for x := 0; x < defaultTexW; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			t := 1.0 - d/maxR
			if t < 0 {
				t = 0
			}
			a := uint8((t * t) * 255)
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, a})
		}
	}
	fireImage = ebiten.NewImageFromImage(img)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	_ = os.WriteFile("fallback_fire.png", buf.Bytes(), 0644)

	fireImageW = float64(fireImage.Bounds().Dx())
	fireImageH = float64(fireImage.Bounds().Dy())
}

type Particle struct {
	x, y, z           float64
	vx, vy, vz        float64
	lifetime, maxLife int
	baseScale         float64
	angle             float64
	angularVelocity   float64
	active            bool
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
	p.z += p.vz

	p.angle += p.angularVelocity
	p.vy += 0.02 // gentle upward drift
	p.vz *= 0.98 // slow damping in depth
}

type Game struct {
	particles []*Particle
	vertices  []ebiten.Vertex
	indices   []uint16
}

func NewGame() *Game {
	g := &Game{
		particles: make([]*Particle, 0, maxParticles),
		vertices:  make([]ebiten.Vertex, 0, maxParticles*4),
		indices:   make([]uint16, 0, maxParticles*6),
	}
	for i := 0; i < maxParticles; i++ {
		g.particles = append(g.particles, &Particle{})
	}
	return g
}

func (g *Game) allocateParticle() *Particle {
	for _, p := range g.particles {
		if !p.active {
			return p
		}
	}
	return nil
}

func newFireParticle(x, y float64) *Particle {
	p := &Particle{
		active:          true,
		x:               x + rand.Float64()*4 - 2,
		y:               y + rand.Float64()*4 - 2,
		z:               rand.Float64()*2 - 1, // depth
		angle:           rand.Float64() * 2 * math.Pi,
		angularVelocity: (rand.Float64()*2 - 1) * 0.1,
		maxLife:         rand.Intn(40) + 40,
		baseScale:       rand.Float64()*0.1 + 0.2,
	}
	ang := rand.Float64() * 2 * math.Pi
	speed := rand.Float64()*4.0 + 2.0
	p.vx = math.Cos(ang) * speed * 0.3
	p.vy = math.Sin(ang) * speed * 0.7
	p.vz = (rand.Float64()*2 - 1) * 0.5
	return p
}

func (g *Game) spawnExplosion(x, y float64) {
	for i := 0; i < 600; i++ {
		if p := g.allocateParticle(); p != nil {
			*p = *newFireParticle(x, y)
		} else {
			break
		}
	}
}

// Blue (far) → Red (near)
func depthColor(z float64) (r, g, b float32) {
	// Normalize z from -2 (far) to +2 (near)
	t := float32((z + 2.0) / 4.0)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// Interpolate blue → purple → red
	r = t
	g = 0.0
	b = 1.0 - t
	return
}

func (g *Game) Update() error {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		g.spawnExplosion(float64(mx), float64(my))
	}

	for _, p := range g.particles {
		if p.active {
			p.update()
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 10, 20, 255})

	g.vertices = g.vertices[:0]
	g.indices = g.indices[:0]
	fireVertexCount := 0

	sx0, sy0 := 0.0, 0.0
	sx1, sy1 := fireImageW, fireImageH
	halfW, halfH := fireImageW/2.0, fireImageH/2.0

	for _, p := range g.particles {
		if !p.active {
			continue
		}
		rate := float64(p.lifetime) / float64(p.maxLife)
		alpha := float32(1.0 - math.Pow(rate, 1.5))

		// Perspective scaling based on depth
		depthScale := float64(1.0 / (1.0 + p.z*0.5))
		scale := p.baseScale * (1.0 + 0.5*rate) * depthScale

		// Colorize based on depth
		r, gcol, b := depthColor(p.z)

		var geo ebiten.GeoM
		geo.Translate(-halfW, -halfH)
		geo.Rotate(p.angle)
		geo.Scale(scale, scale)
		geo.Translate(p.x, p.y)

		vIndex := uint16(fireVertexCount)
		fireVertexCount += 4
		corners := []struct{ dx, dy, sx, sy float64 }{
			{0, 0, sx0, sy0},
			{0, fireImageH, sx0, sy1},
			{fireImageW, 0, sx1, sy0},
			{fireImageW, fireImageH, sx1, sy1},
		}
		for _, c := range corners {
			vx, vy := geo.Apply(c.dx, c.dy)
			g.vertices = append(g.vertices, ebiten.Vertex{
				DstX: float32(vx), DstY: float32(vy),
				SrcX: float32(c.sx), SrcY: float32(c.sy),
				ColorR: r * alpha,
				ColorG: gcol * alpha,
				ColorB: b * alpha,
				ColorA: alpha,
			})
		}
		g.indices = append(g.indices, vIndex, vIndex+1, vIndex+2, vIndex+1, vIndex+3, vIndex+2)
	}

	if len(g.vertices) > 0 && len(g.indices) > 0 {
		op := &ebiten.DrawTrianglesOptions{CompositeMode: ebiten.CompositeModeLighter}
		screen.DrawTriangles(g.vertices, g.indices, fireImage, op)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("Particles: %d/%d\n[LMB] Explosion (Depth Color: Blue→Red)", len(g.vertices)/4, maxParticles))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("🔥 3D Depth Fire Particles (Blue→Red)")
	ebiten.SetTPS(60)
	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
