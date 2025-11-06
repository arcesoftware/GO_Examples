package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/images"
)

const (
	screenWidth  = 1024
	screenHeight = 768
	maxParticles = 1200
	spawnPerTick = 8
	focalLength  = 450.0 // controls perspective strength
	worldRadius  = 220.0 // size of the particle cloud
)

var smokeImage *ebiten.Image

func init() {
	rand.Seed(time.Now().UnixNano())
	img, _, err := image.Decode(bytes.NewReader(images.Smoke_png))
	if err != nil {
		log.Fatal(err)
	}
	smokeImage = ebiten.NewImageFromImage(img)
}

// Particle holds a simple 3D particle
type Particle struct {
	// 3D position
	x, y, z float64
	// 3D velocity
	vx, vy, vz float64

	angle float64
	spin  float64

	baseScale float64

	life    int
	maxLife int

	colorMix color.RGBA
	img      *ebiten.Image
}

// NewParticle creates a particle inside a spherical cloud around origin
func NewParticle(img *ebiten.Image) *Particle {
	// random point in sphere
	phi := rand.Float64() * 2 * math.Pi
	costheta := rand.Float64()*2 - 1
	u := rand.Float64()
	r := worldRadius * math.Cbrt(u) // uniform in sphere by cube root

	x := r * math.Cos(phi) * math.Sqrt(1-costheta*costheta)
	y := r * math.Sin(phi) * math.Sqrt(1-costheta*costheta)
	z := r * costheta

	// small random outward velocity
	speed := rand.Float64()*0.6 + 0.1
	vx := x / (worldRadius + 1) * speed * 0.8
	vy := y / (worldRadius + 1) * speed * 0.8
	vz := z / (worldRadius + 1) * speed * 0.8

	maxLife := 80 + rand.Intn(160)

	return &Particle{
		x:         x,
		y:         y,
		z:         z,
		vx:        vx,
		vy:        vy,
		vz:        vz,
		angle:     rand.Float64() * 2 * math.Pi,
		spin:      (rand.Float64()*2 - 1) * 0.05,
		baseScale: rand.Float64()*0.18 + 0.12,
		life:      maxLife,
		maxLife:   maxLife,
		colorMix:  color.RGBA{uint8(180 + rand.Intn(60)), uint8(180 + rand.Intn(60)), 255, 255},
		img:       img,
	}
}

func (p *Particle) update() bool {
	// simple motion; slight drift and damping
	p.x += p.vx
	p.y += p.vy
	p.z += p.vz

	// tiny inward pull to keep cloud cohesive
	p.vx *= 0.995
	p.vy *= 0.995
	p.vz *= 0.995

	// life
	p.life--
	p.angle += p.spin

	return p.life > 0
}

// projected returns screen x,y, scale, and depth (used for sorting).
// cameraYaw and cameraPitch rotate the world before projection.
func (p *Particle) projected(cameraYaw, cameraPitch float64) (sx, sy, scale, depth float64, visible bool) {
	// rotate around Y (yaw) then X (pitch)
	// rotation around Y:
	siny := math.Sin(cameraYaw)
	cosy := math.Cos(cameraYaw)
	x1 := p.x*cosy + p.z*siny
	z1 := -p.x*siny + p.z*cosy

	// rotation around X (pitch)
	sinp := math.Sin(cameraPitch)
	cosp := math.Cos(cameraPitch)
	y1 := p.y*cosp - z1*sinp
	z2 := p.y*sinp + z1*cosp

	// translate camera a bit back so particles are in front
	z2 += 600 // move camera behind origin (increase for more depth)

	// if behind camera or too close, not visible
	if z2 <= 10 {
		return 0, 0, 0, z2, false
	}

	// perspective projection
	f := focalLength / z2
	screenX := x1*f + screenWidth/2.0
	screenY := y1*f + screenHeight/2.0

	// scale by perspective and baseScale
	scale = p.baseScale * f * 2.0 // multiplier to get pleasant sizes

	// optionally clamp values for safety
	if scale <= 0 || scale > 10 {
		// still visible (but maybe very small/large); we can allow small values
	}

	// depth used for sorting: larger depth => farther from camera
	depth = z2

	return screenX, screenY, scale, depth, true
}

type Game struct {
	particles   []*Particle
	tick        int
	cameraYaw   float64
	cameraPitch float64
}

func (g *Game) spawn(n int) {
	for i := 0; i < n && len(g.particles) < maxParticles; i++ {
		g.particles = append(g.particles, NewParticle(smokeImage))
	}
}

func (g *Game) Update() error {
	g.tick++
	// spawn
	if g.tick%2 == 0 {
		g.spawn(spawnPerTick)
	}

	// animate camera slowly
	g.cameraYaw += 0.004
	g.cameraPitch = math.Sin(float64(g.tick)*0.002) * 0.15

	// update particles and compact slice in place
	write := 0
	for _, p := range g.particles {
		if p.update() {
			g.particles[write] = p
			write++
		}
	}
	g.particles = g.particles[:write]

	// occasionally inject new ones from center so cloud regenerates
	if len(g.particles) < maxParticles/3 {
		g.spawn(40)
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// background gradient-ish fill (single color for simplicity)
	screen.Fill(color.RGBA{10, 14, 28, 255})

	type drawItem struct {
		p         *Particle
		sx, sy    float64
		scale     float64
		depth     float64
		alphaMult float64
	}

	items := make([]drawItem, 0, len(g.particles))

	// Project particles and collect draw items
	for _, p := range g.particles {
		sx, sy, scale, depth, ok := p.projected(g.cameraYaw, g.cameraPitch)
		if !ok {
			continue
		}
		// life-based fade (0..1)
		lifeRatio := float64(p.life) / float64(p.maxLife)
		// depth-based fade to simulate atmospheric depth (farther => dimmer)
		depthFade := 1.0 - (depth-200.0)/(1200.0) // tweak constants for desired look
		if depthFade < 0.25 {
			depthFade = 0.25
		}
		alpha := lifeRatio * depthFade

		items = append(items, drawItem{
			p:         p,
			sx:        sx,
			sy:        sy,
			scale:     scale,
			depth:     depth,
			alphaMult: alpha,
		})
	}

	// depth sort: far -> near (draw far first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].depth > items[j].depth // larger depth = farther
	})

	// draw items: farther first so nearer draw last (occlude)
	for _, it := range items {
		op := &ebiten.DrawImageOptions{}
		w, h := it.p.img.Bounds().Dx(), it.p.img.Bounds().Dy()
		op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
		// rotate with particle angle for visual variety
		op.GeoM.Rotate(it.p.angle)
		op.GeoM.Scale(it.scale, it.scale)
		op.GeoM.Translate(it.sx, it.sy)

		// color scale + alpha; base color + life/depth alpha
		// Compute normalized components as float64 and clamp alpha into [0,1].
		rf := float64(it.p.colorMix.R) / 255.0
		gf := float64(it.p.colorMix.G) / 255.0
		bf := float64(it.p.colorMix.B) / 255.0
		a := it.alphaMult
		if a < 0 {
			a = 0
		} else if a > 1 {
			a = 1
		}
		// Use a temporary ColorM to set the color (ColorM.Scale expects float64 in this ebiten version)
		cs := &ebiten.ColorM{}
		cs.Scale(rf, gf, bf, a)
		op.ColorM = *cs

		screen.DrawImage(it.p.img, op)
	}

	// HUD
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Particles: %d\nTPS: %.2f", len(g.particles), ebiten.ActualTPS()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("3D-like Particles - Depth-sorted (Ebiten)")
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
