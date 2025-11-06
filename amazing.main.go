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
	screenWidth   = 1280
	screenHeight  = 720
	maxParticles  = 14000 // pooled capacity
	defaultTexW   = 36
	defaultTexH   = 36
	maxVertices   = maxParticles * 4
	maxIndices    = maxParticles * 6
	maxEmitters   = 10
	spawnPerFrame = 200 // soft cap (emitters modulate actual spawns)
)

var (
	fireImage  *ebiten.Image
	fireImageW float64
	fireImageH float64
)

func init() {
	rand.Seed(time.Now().UnixNano())

	// Procedural circular alpha texture (soft)
	img := image.NewRGBA(image.Rect(0, 0, defaultTexW, defaultTexH))
	cx, cy := float64(defaultTexW)/2.0, float64(defaultTexH)/2.0
	maxR := math.Hypot(cx, cy)
	for y := 0; y < defaultTexH; y++ {
		for x := 0; x < defaultTexW; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			t := 1.0 - d/maxR
			if t < 0 {
				t = 0
			}
			// sharpen center a bit and soften edges
			a := uint8(math.Pow(t, 1.4) * 255)
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

// Particle types: two flavors for variety
type PKind int

const (
	KindFire PKind = iota
	KindEmber
)

type Particle struct {
	x, y, z           float64
	vx, vy, vz        float64
	lifetime, maxLife int
	baseScale         float64
	angle             float64
	angularVelocity   float64
	kind              PKind
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

	// natural forces vary by kind
	if p.kind == KindFire {
		// slight upward acceleration and drag
		p.vy -= 0.015
		p.vx *= 0.998
		p.vy *= 0.999
		p.vz *= 0.994
	} else {
		// embers: float upwards slowly, fade with wobble
		p.vy -= 0.01
		p.vx += (rand.Float64()*2 - 1) * 0.02
		p.vz *= 0.995
	}
}

// Emitter: autonomous, moves along a path and pulses
type Emitter struct {
	cx, cy     float64 // center of orbit
	radius     float64
	phase      float64
	speed      float64
	baseSpawn  int     // base spawn per pulse
	pulseWidth float64 // pulse frequency component
	kind       PKind
	offsetY    float64 // vertical offset for layout
}

type Game struct {
	particles []*Particle
	vertices  []ebiten.Vertex
	indices   []uint16

	emitters []*Emitter
	tick     int64

	// camera parallax wobble
	depthOffset float64
}

func NewGame() *Game {
	g := &Game{
		particles: make([]*Particle, 0, maxParticles),
		vertices:  make([]ebiten.Vertex, 0, maxVertices),
		indices:   make([]uint16, 0, maxIndices),
		emitters:  make([]*Emitter, 0, maxEmitters),
	}

	// prefill pool
	for i := 0; i < maxParticles; i++ {
		g.particles = append(g.particles, &Particle{})
	}

	// configure a few moving emitters across the screen
	for i := 0; i < 6; i++ {
		a := rand.Float64() * 2 * math.Pi
		r := 120.0 + rand.Float64()*420.0
		cx := screenWidth/2.0 + rand.Float64()*200.0 - 100.0
		cy := screenHeight/2.0 + rand.Float64()*120.0 - 60.0
		e := &Emitter{
			cx:         cx,
			cy:         cy,
			radius:     r,
			phase:      a,
			speed:      0.002 + rand.Float64()*0.006,
			baseSpawn:  6 + rand.Intn(12),
			pulseWidth: 0.8 + rand.Float64()*1.8,
			kind:       KindFire,
			offsetY:    rand.Float64()*40 - 20,
		}
		g.emitters = append(g.emitters, e)
	}

	// a couple of ember-focused emitters for long tails
	for i := 0; i < 3; i++ {
		e := &Emitter{
			cx:         float64(screenWidth) * (0.2 + rand.Float64()*0.6),
			cy:         float64(screenHeight) * (0.6 + rand.Float64()*0.2),
			radius:     10 + rand.Float64()*60,
			phase:      rand.Float64() * 2 * math.Pi,
			speed:      0.001 + rand.Float64()*0.004,
			baseSpawn:  2 + rand.Intn(3),
			pulseWidth: 3.0 + rand.Float64()*6.0,
			kind:       KindEmber,
			offsetY:    0,
		}
		g.emitters = append(g.emitters, e)
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

func (g *Game) spawnAt(x, y float64, kind PKind) {
	// spawn a single particle of given kind with random variation
	if p := g.allocateParticle(); p != nil {
		*p = Particle{}
		p.active = true
		p.kind = kind
		p.x = x + (rand.Float64()*2-1)*6
		p.y = y + (rand.Float64()*2-1)*6
		// depth placed slightly in front/behind for spread
		p.z = rand.Float64()*2.2 - 1.0
		p.angle = rand.Float64() * 2 * math.Pi
		p.angularVelocity = (rand.Float64()*2 - 1) * 0.12

		if kind == KindFire {
			p.maxLife = 30 + rand.Intn(50)
			p.baseScale = 0.14 + rand.Float64()*0.22
			ang := rand.Float64() * 2 * math.Pi
			speed := 1.2 + rand.Float64()*5.8
			p.vx = math.Cos(ang) * speed * (0.2 + rand.Float64()*0.6)
			p.vy = math.Sin(ang) * speed * (0.3 + rand.Float64()*0.9)
			p.vz = rand.Float64()*1.2 - 0.6
		} else {
			// ember: smaller, longer lived, slower
			p.maxLife = 120 + rand.Intn(200)
			p.baseScale = 0.05 + rand.Float64()*0.08
			p.vx = (rand.Float64()*2 - 1) * 0.6
			p.vy = -0.2 - rand.Float64()*0.6
			p.vz = (rand.Float64()*2 - 1) * 0.15
			p.angularVelocity = (rand.Float64()*2 - 1) * 0.03
		}
	}
}

func (g *Game) spawnBurst(x, y float64, count int) {
	for i := 0; i < count; i++ {
		g.spawnAt(x, y, KindFire)
	}
}

// depthColor: blue (far) -> purple -> red (near) with small time hue shift
func depthColor(z float64, t float64) (r, g, b float32) {
	// Normalize z from -2 (far) to +2 (near)
	nt := float64((z + 2.0) / 4.0)
	if nt < 0 {
		nt = 0
	}
	if nt > 1 {
		nt = 1
	}
	// add slow hue shift for spectacle
	shift := 0.15 * math.Sin(t*0.8)
	tt := nt + shift
	if tt < 0 {
		tt = 0
	}
	if tt > 1 {
		tt = 1
	}
	// smooth interpolation through blue->magenta->red
	// use sinusoidal ease for nicer transitions
	s := math.Sin(tt * math.Pi / 2) // 0..1
	r = float32(s)
	g = float32((1 - tt) * 0.25) // slight green tint in mid
	b = float32(1 - s)
	// boost saturation for near particles
	if tt > 0.6 {
		r = float32(math.Min(1.0, float64(r)*1.15))
	}
	return
}

func (g *Game) Update() error {
	g.tick++

	// input: left click still does a big burst
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		// big synchronized burst
		g.spawnBurst(float64(mx), float64(my), 900)
	}

	// press space for random super-burst
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		px := float64(rand.Intn(screenWidth))
		py := float64(rand.Intn(screenHeight/2) + screenHeight/3)
		g.spawnBurst(px, py, 1200)
	}

	// autonomous emitters: move them and spawn based on sine pulses
	now := float64(g.tick) / 60.0 // seconds elapsed
	totalSpawns := 0
	for _, e := range g.emitters {
		e.phase += e.speed
		// compute emitter position on a circular orbit
		angle := e.phase*2*math.Pi + e.phase*1.1
		ex := e.cx + math.Cos(angle)*e.radius
		ey := e.cy + math.Sin(angle*0.9)*e.radius*0.55 + e.offsetY

		// pulse factor (0..1)
		pulse := (math.Sin(now*e.pulseWidth+e.phase*4.0) + 1.0) * 0.5
		// jittered spawn count
		target := int(float64(e.baseSpawn) * (0.5 + pulse) * (0.8 + rand.Float64()*0.8))
		if e.kind == KindEmber {
			// embers spawn slowly
			target = int(float64(e.baseSpawn) * (0.2 + pulse*0.5))
		}
		// cap per-emitter to avoid pool exhaustion
		if target > 250 {
			target = 250
		}
		for i := 0; i < target && totalSpawns < spawnPerFrame; i++ {
			// pseudorandom small jitter around emitter
			jx := ex + (rand.Float64()*2-1)*20
			jy := ey + (rand.Float64()*2-1)*20
			g.spawnAt(jx, jy, e.kind)
			totalSpawns++
		}

		// occasional surprise burst
		if rand.Float64() < 0.003 {
			g.spawnBurst(ex, ey, 220+rand.Intn(480))
		}
	}

	// small global camera depth offset wobble for parallax
	g.depthOffset = 0.18 * math.Sin(now*0.25)

	// update particles
	for _, p := range g.particles {
		if p.active {
			p.update()
			// recycle if off screen far away
			if p.x < -200 || p.x > screenWidth+200 || p.y < -300 || p.y > screenHeight+400 {
				p.active = false
			}
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// nice dark radial background gradient
	bg := color.RGBA{10, 6, 26, 255}
	screen.Fill(bg)

	// subtle vignette: draw a semi-transparent rectangle overlay for concert look
	overlay := ebiten.NewImage(screenWidth, screenHeight)
	overlay.Fill(color.RGBA{0, 0, 0, 40})
	screen.DrawImage(overlay, nil)

	// prepare buffers (reuse slices)
	g.vertices = g.vertices[:0]
	g.indices = g.indices[:0]
	fireVertexCount := 0

	now := float64(g.tick) / 60.0

	sx0, sy0 := 0.0, 0.0
	sx1, sy1 := fireImageW, fireImageH
	halfW, halfH := fireImageW/2.0, fireImageH/2.0

	// draw a faint starfield (cheap)
	if (g.tick % 30) == 0 {
		// occasionally add a twinkling star (just draw small points)
		x := rand.Float64() * screenWidth
		y := rand.Float64() * screenHeight * 0.6
		ebitenutil.DrawRect(screen, x, y, 2, 2, color.RGBA{200, 200, 255, 60})
	}

	for _, p := range g.particles {
		if !p.active {
			continue
		}
		rate := float64(p.lifetime) / float64(p.maxLife)
		// depth adjusted by camera offset
		z := p.z + g.depthOffset
		alpha := float32((1.0 - math.Pow(rate, 1.4)) * (0.20 + (1.0-math.Abs(z))*0.85))
		if alpha < 0 {
			alpha = 0
		}
		// perspective scaling: near particles bigger
		depthScale := 1.0 / (1.0 + z*0.6)
		if depthScale < 0.3 {
			depthScale = 0.3
		}
		scale := p.baseScale * (1.0 + 0.8*rate) * depthScale

		// color by depth + time
		rcol, gcol, bcol := depthColor(z, now)

		// brighter for fire, dim for embers
		if p.kind == KindEmber {
			alpha *= 0.7
			scale *= 0.6
		} else {
			alpha = float32(math.Min(1.0, float64(alpha)*1.15))
		}

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
				ColorR: rcol * alpha,
				ColorG: gcol * alpha,
				ColorB: bcol * alpha,
				ColorA: alpha,
			})
		}
		g.indices = append(g.indices, vIndex, vIndex+1, vIndex+2, vIndex+1, vIndex+3, vIndex+2)
	}

	// Draw all particles with additive blending for glow
	if len(g.vertices) > 0 && len(g.indices) > 0 {
		op := &ebiten.DrawTrianglesOptions{CompositeMode: ebiten.CompositeModeLighter}
		screen.DrawTriangles(g.vertices, g.indices, fireImage, op)
	}

	// HUD: simple status for live shows
	activeCount := 0
	for _, p := range g.particles {
		if p.active {
			activeCount++
		}
	}
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Particles: %d/%d  |  Emitters: %d  |  [LMB]=burst  [SPACE]=superburst", activeCount, maxParticles, len(g.emitters)))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Concert Particle Show — Live Mode")
	ebiten.SetTPS(60)
	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
