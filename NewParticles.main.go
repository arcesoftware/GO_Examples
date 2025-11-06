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
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/images"
)

const (
	screenWidth  = 800
	screenHeight = 600
	maxParticles = 800
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

type Particle struct {
	x, y     float64
	vx, vy   float64
	angle    float64
	scale    float64
	alpha    float32
	life     int
	maxLife  int
	img      *ebiten.Image
	colorMix color.RGBA
}

func NewParticle(img *ebiten.Image) *Particle {
	dir := rand.Float64() * 2 * math.Pi
	speed := rand.Float64()*1.5 + 0.5

	return &Particle{
		x:        screenWidth / 2,
		y:        screenHeight / 2,
		vx:       math.Cos(dir) * speed,
		vy:       math.Sin(dir) * speed,
		angle:    rand.Float64() * 2 * math.Pi,
		scale:    rand.Float64()*0.2 + 0.3,
		alpha:    0.6,
		life:     60 + rand.Intn(120),
		maxLife:  60 + rand.Intn(120),
		img:      img,
		colorMix: color.RGBA{uint8(200 + rand.Intn(55)), uint8(200 + rand.Intn(55)), 255, 255},
	}
}

func (p *Particle) Update() bool {
	p.x += p.vx
	p.y += p.vy
	p.vy += 0.01 // light upward drift or gravity effect tweak

	p.angle += 0.01
	p.life--

	return p.life > 0
}

func (p *Particle) Draw(screen *ebiten.Image) {
	if p.life <= 0 {
		return
	}

	ratio := float32(p.life) / float32(p.maxLife)
	alpha := p.alpha * ratio
	if alpha < 0 {
		alpha = 0
	}

	op := &ebiten.DrawImageOptions{}
	w, h := p.img.Bounds().Dx(), p.img.Bounds().Dy()

	op.GeoM.Translate(-float64(w)/2, -float64(h)/2)
	op.GeoM.Rotate(p.angle)
	op.GeoM.Scale(p.scale, p.scale)
	op.GeoM.Translate(p.x, p.y)
	op.ColorScale.Scale(float32(p.colorMix.R)/255, float32(p.colorMix.G)/255, float32(p.colorMix.B)/255, alpha)

	screen.DrawImage(p.img, op)
}

type Game struct {
	particles []*Particle
	tick      int
}

func (g *Game) Update() error {
	// Spawn new particles periodically
	if len(g.particles) < maxParticles && g.tick%2 == 0 {
		for i := 0; i < 5; i++ {
			g.particles = append(g.particles, NewParticle(smokeImage))
		}
	}
	g.tick++

	// Update particles and compact slice
	n := 0
	for _, p := range g.particles {
		if p.Update() {
			g.particles[n] = p
			n++
		}
	}
	g.particles = g.particles[:n]
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x10, 0x18, 0x30, 0xff})
	for _, p := range g.particles {
		p.Draw(screen)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %.2f\nParticles: %d", ebiten.ActualTPS(), len(g.particles)))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Modern Particle System (Ebiten)")
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
