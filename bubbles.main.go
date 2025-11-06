package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	screenWidth  = 1024
	screenHeight = 768
	maxParticles = 1200
	spawnPerTick = 8
	focalLength  = 450.0
	worldRadius  = 220.0
)

type Particle struct {
	x, y, z float64
	vx, vy, vz float64
	life, maxLife int
	baseSize float64
	color    color.RGBA
}

func NewParticle() *Particle {
	phi := rand.Float64() * 2 * math.Pi
	costheta := rand.Float64()*2 - 1
	u := rand.Float64()
	r := worldRadius * math.Cbrt(u)

	x := r * math.Cos(phi) * math.Sqrt(1-costheta*costheta)
	y := r * math.Sin(phi) * math.Sqrt(1-costheta*costheta)
	z := r * costheta

	speed := rand.Float64()*1.5 + 0.5
	vx := x / (worldRadius+1) * speed * 0.5
	vy := y / (worldRadius+1) * speed * 0.5
	vz := z / (worldRadius+1) * speed * 0.5

	maxLife := 100 + rand.Intn(120)
	col := color.RGBA{
		uint8(180 + rand.Intn(70)),
		uint8(180 + rand.Intn(70)),
		uint8(255),
		255,
	}

	return &Particle{
		x: x, y: y, z: z,
		vx: vx, vy: vy, vz: vz,
		life: maxLife, maxLife: maxLife,
		baseSize: rand.Float64()*3 + 2,
		color: col,
	}
}

func (p *Particle) Update() bool {
	p.x += p.vx
	p.y += p.vy
	p.z += p.vz
	p.vx *= 0.99
	p.vy *= 0.99
	p.vz *= 0.99
	p.life--
	return p.life > 0
}

func (p *Particle) Project(yaw, pitch float64) (sx, sy, scale, depth float64, visible bool) {
	siny, cosy := math.Sin(yaw), math.Cos(yaw)
	x1 := p.x*cosy + p.z*siny
	z1 := -p.x*siny + p.z*cosy

	sinp, cosp := math.Sin(pitch), math.Cos(pitch)
	y1 := p.y*cosp - z1*sinp
	z2 := p.y*sinp + z1*cosp + 600 // camera offset

	if z2 <= 10 {
		return 0, 0, 0, z2, false
	}

	f := focalLength / z2
	sx = x1*f + screenWidth/2
	sy = y1*f + screenHeight/2
	scale = f
	depth = z2
	return sx, sy, scale, depth, true
}

type Game struct {
	particles []*Particle
	tick int
	yaw, pitch float64
}

func (g *Game) spawn(n int) {
	for i := 0; i < n && len(g.particles) < maxParticles; i++ {
		g.particles = append(g.particles, NewParticle())
	}
}

func (g *Game) Update() error {
	g.tick++
	if g.tick%2 == 0 {
		g.spawn(spawnPerTick)
	}
	g.yaw += 0.004
	g.pitch = math.Sin(float64(g.tick)*0.002) * 0.15

	write := 0
	for _, p := range g.particles {
		if p.Update() {
			g.particles[write] = p
			write++
		}
	}
	g.particles = g.particles[:write]
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 14, 28, 255})

	type drawItem struct {
		x, y, size, depth, alpha float64
		col                      color.RGBA
	}
	items := make([]drawItem, 0, len(g.particles))

	for _, p := range g.particles {
		sx, sy, scale, depth, ok := p.Project(g.yaw, g.pitch)
		if !ok {
			continue
		}
		lifeRatio := float64(p.life) / float64(p.maxLife)
		depthFade := 1.0 - (depth-200)/1200
		if depthFade < 0.2 {
			depthFade = 0.2
		}
		alpha := lifeRatio * depthFade
		size := p.baseSize * scale * 3.0

		items = append(items, drawItem{sx, sy, size, depth, alpha, p.color})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].depth > items[j].depth })

	for _, it := range items {
		c := it.col
		a := uint8(255 * it.alpha)
		if a < 10 {
			continue
		}
		c.A = a
		vector.DrawFilledCircle(screen, float32(it.x), float32(it.y), float32(it.size), c, true)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("Particles: %d\nTPS: %.2f", len(g.particles), ebiten.ActualTPS()))
}

func (g *Game) Layout(ow, oh int) (int, int) { return screenWidth, screenHeight }

func main() {
	rand.Seed(time.Now().UnixNano())
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("3D Procedural Particles (Ebiten)")
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
