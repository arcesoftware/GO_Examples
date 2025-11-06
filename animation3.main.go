package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
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
	// Use math/rand for seeding, but we will use rand.Float64() for values.
	rand.Seed(time.Now().UnixNano()) 

	// Procedural circular alpha texture (A soft, fading circle for glow)
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
			// Use a squared falloff for a softer glow
			a := uint8((t * t) * 255) 
			// White base color, the actual color will be tinted during drawing
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, a}) 
		}
	}
	fireImage = ebiten.NewImageFromImage(img)
	fireImageW = float64(fireImage.Bounds().Dx())
	fireImageH = float64(fireImage.Bounds().Dy())
}

// Particle represents a single element in the system.
type Particle struct {
	x, y, z             float64
	vx, vy, vz          float64
	lifetime, maxLife   int
	baseScale           float64
	angle               float64
	angularVelocity     float64
	active              bool
}

// update handles the physics and life of the particle.
func (p *Particle) update() {
	if !p.active {
		return
	}
	p.lifetime++
	if p.lifetime >= p.maxLife {
		p.active = false
		return
	}

	// Apply physics: movement, gentle upward drift (Y), and damping (Z)
	p.x += p.vx
	p.y += p.vy
	p.z += p.vz

	p.angle += p.angularVelocity
	p.vy += 0.02 
	p.vz *= 0.98 
}

// Game holds the main state and resources.
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
	// Initialize object pool
	for i := 0; i < maxParticles; i++ {
		g.particles = append(g.particles, &Particle{})
	}
	return g
}

// allocateParticle finds the next available (inactive) particle from the pool.
func (g *Game) allocateParticle() *Particle {
	for _, p := range g.particles {
		if !p.active {
			return p
		}
	}
	return nil
}

// newFireParticle initializes a particle with explosion-specific properties.
func newFireParticle(x, y float64) *Particle {
	p := &Particle{
		active:          true,
		x:               x + rand.Float64()*4 - 2,
		y:               y + rand.Float64()*4 - 2,
		z:               rand.Float64()*2 - 1, // Start depth: -1 (far) to +1 (near)
		angle:           rand.Float64() * 2 * math.Pi,
		angularVelocity: (rand.Float64()*2 - 1) * 0.1,
		maxLife:         rand.Intn(40) + 40,
		baseScale:       rand.Float64()*0.1 + 0.2,
	}
	// Radial outward velocity for explosion
	ang := rand.Float64() * 2 * math.Pi
	speed := rand.Float64()*4.0 + 2.0
	p.vx = math.Cos(ang) * speed * 0.3
	p.vy = math.Sin(ang) * speed * 0.7 
	p.vz = (rand.Float64()*2 - 1) * 0.5
	return p
}

// spawnExplosion creates a large burst of particles at the given screen coordinates.
func (g *Game) spawnExplosion(x, y float64) {
	// Spawn 600 particles per click
	for i := 0; i < 600; i++ {
		if p := g.allocateParticle(); p != nil {
			*p = *newFireParticle(x, y)
		} else {
			break
		}
	}
}

func (g *Game) Update() error {
	// Handle input: Left Mouse Button spawns an explosion
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		g.spawnExplosion(float64(mx), float64(my))
	}

	// Update all active particles
	for _, p := range g.particles {
		if p.active {
			p.update()
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Dark background for maximum glow contrast
	screen.Fill(color.RGBA{10, 10, 20, 255}) 

	g.vertices = g.vertices[:0]
	g.indices = g.indices[:0]
	fireVertexCount := 0

	sx0, sy0 := 0.0, 0.0
	sx1, sy1 := fireImageW, fireImageH
	halfW, halfH := fireImageW/2.0, fireImageH/2.0

	// Sort particles by Z-depth to ensure correct drawing order (near particles draw last)
	// NOTE: This simple sort is only for visual accuracy and can be costly, but is crucial for 3D illusion.
	// For massive scale, sorting would be optional or handled with a depth buffer.
	activeParticles := make([]*Particle, 0, len(g.particles))
	for _, p := range g.particles {
		if p.active {
			activeParticles = append(activeParticles, p)
		}
	}
	// Sort near-to-far so that the particles are drawn far-to-near (painter's algorithm)
	for i := range activeParticles {
		for j := i + 1; j < len(activeParticles); j++ {
			if activeParticles[i].z > activeParticles[j].z {
				activeParticles[i], activeParticles[j] = activeParticles[j], activeParticles[i]
			}
		}
	}

	for _, p := range activeParticles {
		rate := float64(p.lifetime) / float64(p.maxLife)
		
		// --- 1. Lifetime Color Transition (Blue -> Yellow) ---
		// rate = 0 (Start) -> R=0.0, G=0.0, B=1.0 (Pure Blue)
		// rate = 1 (End)   -> R=1.0, G=1.0, B=0.0 (Pure Yellow)
		r := float32(rate)           // Red increases with life (0 -> 1)
		gcol := float32(rate)        // Green increases with life (0 -> 1) <--- MODIFIED LINE
		b := float32(1.0 - rate)     // Blue decreases with life (1 -> 0)
		
		// Alpha fade out (Exponential fade for a quick dissipation)
		alpha := float32(1.0 - math.Pow(rate, 1.5)) 

		// --- 2. 3D Scaling (Depth) ---
		// Far (negative Z) particles are smaller; Near (positive Z) particles are larger.
		// The scale is combined with the growth over life.
		depthScale := float64(1.0 / (1.0 + p.z*0.5)) // Simple perspective scale
		scale := p.baseScale * (1.0 + 0.5*rate) * depthScale

		// --- 3. Geometry Calculation ---
		var geo ebiten.GeoM
		geo.Translate(-halfW, -halfH)
		geo.Rotate(p.angle)
		geo.Scale(scale, scale)
		geo.Translate(p.x, p.y)

		// --- 4. Batching Vertices ---
		vIndex := uint16(fireVertexCount)
		fireVertexCount += 4
		
		// Map texture coordinates (SrcX/Y) to screen coordinates (DstX/Y)
		corners := []struct{ dx, dy, sx, sy float64 }{
			{0, 0, sx0, sy0},
			{0, fireImageH, sx0, sy1},
			{fireImageW, 0, sx1, sy0},
			{fireImageW, fireImageH, sx1, sy1},
		}
		for _, c := range corners {
			vx, vy := geo.Apply(c.dx, c.dy)
			// Premultiply color by alpha for correct blending
			g.vertices = append(g.vertices, ebiten.Vertex{
				DstX: float32(vx), DstY: float32(vy),
				SrcX: float32(c.sx), SrcY: float32(c.sy),
				ColorR: r * alpha,
				ColorG: gcol * alpha,
				ColorB: b * alpha,
				ColorA: alpha, // Alpha component is critical for Additive Blending
			})
		}
		// Indices for the two triangles that form the quad
		g.indices = append(g.indices, vIndex, vIndex+1, vIndex+2, vIndex+1, vIndex+3, vIndex+2)
	}

	// --- Final Batch Draw Call ---
	if len(g.vertices) > 0 && len(g.indices) > 0 {
		// CompositeModeLighter is Additive Blending: required for fire/glow effects
		op := &ebiten.DrawTrianglesOptions{CompositeMode: ebiten.CompositeModeLighter}
		screen.DrawTriangles(g.vertices, g.indices, fireImage, op)
	}

	// Debug statistics display
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Particles: %d/%d\n[LMB] Explosion (Color: Blue→Yellow over Life)", len(activeParticles), maxParticles))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("🔥 3D Depth Particles: Lifetime Color Shift (Blue→Yellow)")
	ebiten.SetTPS(60)
	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
