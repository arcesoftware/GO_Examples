package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	_ "image/png"
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
	screenWidth   = 640
	screenHeight  = 480
	maxParticles  = 10000 // safe for uint16 indices (max vertices = 4*maxParticles)
	defaultTexW   = 32
	defaultTexH   = 32
)

var (
	smokeImage    *ebiten.Image
	smokeImageW   float64
	smokeImageH   float64
)

func init() {
	// seed RNG
	rand.Seed(time.Now().UnixNano())

	// Try to load an external image first
	path := "_resources/images/smoke.png"
	if _, err := os.Stat(path); err == nil {
		// file exists
		f, err := os.Open(path)
		if err == nil {
			img, _, err := image.Decode(f)
			_ = f.Close()
			if err == nil {
				smokeImage = ebiten.NewImageFromImage(img)
			}
		}
	}
	// If loading failed, create a small procedural smoke texture (radial alpha)
	if smokeImage == nil {
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
				a := uint8((t * t) * 255) // soft falloff
				// white-ish smoke texture (alpha varies)
				img.SetRGBA(x, y, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: a})
			}
		}
		smokeImage = ebiten.NewImageFromImage(img)
		// Optional: write fallback to disk for debugging
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		_ = os.WriteFile("fallback_smoke.png", buf.Bytes(), 0644)
	}

	smokeImageW = float64(smokeImage.Bounds().Dx())
	smokeImageH = float64(smokeImage.Bounds().Dy())
}

// ParticleType defines the behavior and blending mode.
type ParticleType int

const (
	TypeSmoke ParticleType = iota // Alpha Blending, long life, slow
	TypeFire                      // Additive Blending, short life, high velocity
)

// Particle struct for both smoke and fire.
type Particle struct {
	x, y             float64
	vx, vy           float64
	lifetime         int
	maxLife          int
	baseScale        float64
	angle            float64
	angularVelocity  float64
	col              color.RGBA
	pType            ParticleType
	active           bool
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
	// small upward force/drag typical of smoke/fire
	p.vy += 0.05
}

// Emitter spawns particles at a given rate.
type Emitter struct {
	x, y   float64
	rate   int // spawn every `rate` ticks (1 = every tick)
	pType  ParticleType
	counter int
}

func (e *Emitter) spawn(g *Game) {
	e.counter++
	if e.rate <= 0 {
		e.rate = 1
	}
	if e.counter%e.rate != 0 {
		return
	}
	// burst 2 particles
	for i := 0; i < 2; i++ {
		if p := g.allocateParticle(); p != nil {
			*p = *newParticle(e.x, e.y, e.pType)
		}
	}
}

func newParticle(emitterX, emitterY float64, pType ParticleType) *Particle {
	p := &Particle{
		active: true,
		pType:  pType,
		x:      emitterX + rand.Float64()*4 - 2,
		y:      emitterY + rand.Float64()*4 - 2,
		angle:  rand.Float64() * 2 * math.Pi,
		angularVelocity: (rand.Float64()*2 - 1) * 0.05,
	}
	switch pType {
	case TypeSmoke:
		p.maxLife = rand.Intn(60) + 240 // ~4-5s
		angle := rand.Float64()*math.Pi/3.0 + math.Pi/2.0
		speed := rand.Float64()*0.4 + 0.1
		p.vx = math.Cos(angle) * speed
		p.vy = math.Sin(angle) * speed - 1.0

		r := uint8(0xc0 + rand.Intn(0x3f))
		g := uint8(0xc0 + rand.Intn(0x3f))
		b := uint8(0xc0 + rand.Intn(0x3f))
		p.col = color.RGBA{R: r, G: g, B: b, A: 0xff}
		p.baseScale = rand.Float64()*0.1 + 0.3

	case TypeFire:
		p.maxLife = rand.Intn(30) + 45 // short life
		ang := rand.Float64()*math.Pi/4.0
		if rand.Intn(2) == 0 {
			ang = -ang
		}
		ang += math.Pi / 2.0
		speed := rand.Float64()*1.5 + 1.0
		p.vx = math.Cos(ang) * speed * 0.5
		p.vy = math.Sin(ang) * speed * 2.0

		p.col = color.RGBA{R: 0xff, G: 0x90, B: 0x00, A: 0xff}
		p.baseScale = rand.Float64()*0.05 + 0.15
	}
	return p
}

// Game holds particles, emitters and batching buffers.
type Game struct {
	particles []*Particle
	emitters  []*Emitter

	smokeVertices []ebiten.Vertex
	fireVertices  []ebiten.Vertex
	smokeIndices  []uint16
	fireIndices   []uint16
	// pool cursor not strictly necessary, allocateParticle scans
}

func NewGame() *Game {
	g := &Game{
		particles:     make([]*Particle, 0, maxParticles),
		smokeVertices: make([]ebiten.Vertex, 0, maxParticles*4),
		fireVertices:  make([]ebiten.Vertex, 0, maxParticles*4),
		smokeIndices:  make([]uint16, 0, maxParticles*6),
		fireIndices:   make([]uint16, 0, maxParticles*6),
		emitters:      make([]*Emitter, 0, 4),
	}
	// Pre-create a pool of inactive particles so allocateParticle can reuse without nils.
	for i := 0; i < maxParticles; i++ {
		g.particles = append(g.particles, &Particle{active: false})
	}

	// permanent smoke emitter at bottom-center
	g.emitters = append(g.emitters, &Emitter{
		x:     screenWidth / 2.0,
		y:     screenHeight - 50.0,
		rate:  3,
		pType: TypeSmoke,
	})
	return g
}

func (g *Game) allocateParticle() *Particle {
	for _, p := range g.particles {
		if !p.active {
			return p
		}
	}
	// pool exhausted
	return nil
}

func (g *Game) Update() error {
	// Input: left click to spawn explosion
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		g.spawnExplosion(float64(mx), float64(my))
	}

	// spawn from emitters
	for _, e := range g.emitters {
		e.spawn(g)
	}

	// update particles
	for _, p := range g.particles {
		if p.active {
			p.update()
			// Optionally deactivate particles that go off screen far away
			if p.x < -100 || p.x > screenWidth+100 || p.y < -200 || p.y > screenHeight+200 {
				p.active = false
			}
		}
	}
	return nil
}

func (g *Game) spawnExplosion(x, y float64) {
	// spawn many fire particles in an explosion
	for i := 0; i < 500; i++ {
		if p := g.allocateParticle(); p != nil {
			*p = *newParticle(x, y, TypeFire)
			blastAngle := rand.Float64() * 2 * math.Pi
			blastSpeed := rand.Float64()*7.0 + 3.0
			p.vx = math.Cos(blastAngle) * blastSpeed
			p.vy = math.Sin(blastAngle) * blastSpeed
		} else {
			// pool exhausted; stop spawning
			break
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xff})

	// reset buffers
	g.smokeVertices = g.smokeVertices[:0]
	g.fireVertices = g.fireVertices[:0]
	g.smokeIndices = g.smokeIndices[:0]
	g.fireIndices = g.fireIndices[:0]

	activeCount := 0
	fireVertexCount := 0
	smokeVertexCount := 0

	sx0, sy0 := 0.0, 0.0
	sx1, sy1 := smokeImageW, smokeImageH
	halfW, halfH := smokeImageW/2.0, smokeImageH/2.0

	// iterate particles and push vertices/indices into the correct buffer
	for _, p := range g.particles {
		if !p.active {
			continue
		}
		activeCount++
		rate := float64(p.lifetime) / float64(p.maxLife)
		scale := p.baseScale * (1.0 + 1.0*rate)

		var alpha float32 = 1.0
		if p.pType == TypeFire {
			alpha = float32(1.0 - math.Pow(rate, 2))
		} else { // smoke alpha envelope (fade in, then out)
			if rate < 0.2 {
				alpha = float32(rate / 0.2)
			} else if rate > 0.8 {
				alpha = float32((1 - rate) / 0.2)
			}
		}

		cr := float32(p.col.R) / 0xff * alpha
		cg := float32(p.col.G) / 0xff * alpha
		cb := float32(p.col.B) / 0xff * alpha
		ca := alpha

		// Build GeoM-like transform (apply manually for speed)
		var geo ebiten.GeoM
		geo.Translate(-halfW, -halfH)
		geo.Rotate(p.angle)
		geo.Scale(scale, scale)
		geo.Translate(p.x, p.y)

		// choose target buffer
		if p.pType == TypeFire {
			vIndex := uint16(fireVertexCount)
			fireVertexCount += 4
			// corners: top-left, bottom-left, top-right, bottom-right (matching UV coords)
			corners := []struct{ dx, dy, sx, sy float64 }{
				{0, 0, sx0, sy0},
				{0, smokeImageH, sx0, sy1},
				{smokeImageW, 0, sx1, sy0},
				{smokeImageW, smokeImageH, sx1, sy1},
			}
			for _, c := range corners {
				vx, vy := geo.Apply(c.dx, c.dy)
				g.fireVertices = append(g.fireVertices, ebiten.Vertex{
					DstX:   float32(vx), DstY: float32(vy),
					SrcX:   float32(c.sx), SrcY: float32(c.sy),
					ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
				})
			}
			// two triangles
			g.fireIndices = append(g.fireIndices, vIndex, vIndex+1, vIndex+2, vIndex+1, vIndex+3, vIndex+2)
		} else {
			vIndex := uint16(smokeVertexCount)
			smokeVertexCount += 4
			corners := []struct{ dx, dy, sx, sy float64 }{
				{0, 0, sx0, sy0},
				{0, smokeImageH, sx0, sy1},
				{smokeImageW, 0, sx1, sy0},
				{smokeImageW, smokeImageH, sx1, sy1},
			}
			for _, c := range corners {
				vx, vy := geo.Apply(c.dx, c.dy)
				g.smokeVertices = append(g.smokeVertices, ebiten.Vertex{
					DstX:   float32(vx), DstY: float32(vy),
					SrcX:   float32(c.sx), SrcY: float32(c.sy),
					ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca,
				})
			}
			g.smokeIndices = append(g.smokeIndices, vIndex, vIndex+1, vIndex+2, vIndex+1, vIndex+3, vIndex+2)
		}
	}

	// Draw fire first with additive blending (lighter)
	if len(g.fireVertices) > 0 && len(g.fireIndices) > 0 {
		op := &ebiten.DrawTrianglesOptions{CompositeMode: ebiten.CompositeModeLighter}
		// DrawTriangles expects indices referencing the vertex slice starting at 0.
		screen.DrawTriangles(g.fireVertices, g.fireIndices, smokeImage, op)
	}

	// Draw smoke with normal alpha composite
	if len(g.smokeVertices) > 0 && len(g.smokeIndices) > 0 {
		op := &ebiten.DrawTrianglesOptions{CompositeMode: ebiten.CompositeModeSourceOver}
		screen.DrawTriangles(g.smokeVertices, g.smokeIndices, smokeImage, op)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f\nActive Particles: %d/%d\nLMB: Trigger Explosion",
		ebiten.ActualTPS(), activeCount, maxParticles))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Particle System — smoke & fire (fixed)")
	ebiten.SetTPS(60)

	g := NewGame()

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
