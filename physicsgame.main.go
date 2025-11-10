package main

import (
	"image/color"
	"log"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ============================
// Basic Physics Structures
// ============================

type Vector struct {
	X, Y float64
}

func (v *Vector) Add(o Vector) {
	v.X += o.X
	v.Y += o.Y
}

func (v *Vector) Scale(s float64) {
	v.X *= s
	v.Y *= s
}

func (v Vector) LengthSq() float64 {
	return v.X*v.X + v.Y*v.Y
}

func (v Vector) Length() float64 {
	return math.Sqrt(v.LengthSq())
}

func (v Vector) Normalized() Vector {
	l := v.Length()
	if l == 0 {
		return Vector{}
	}
	return Vector{v.X / l, v.Y / l}
}

// ============================
// Ball and Wall Definitions
// ============================

type Ball struct {
	Pos, Vel Vector
	Radius   float64
	Mass     float64
	Color    color.Color
}

type Wall struct {
	X, Y, W, H float64
	Color      color.Color
}

// ============================
// Simulation Parameters
// ============================

var (
	balls   []*Ball
	walls   []Wall
	dt      = 0.016
	e       = 0.8 // coefficient of restitution
	gravity = Vector{0, 9.8}
	screenW = 800
	screenH = 800
)

// ============================
// Physics Functions
// ============================

func applyForce(b *Ball, f Vector) {
	a := Vector{f.X / b.Mass, f.Y / b.Mass}
	b.Vel.Add(Vector{a.X * dt, a.Y * dt})
}

func updatePosition(b *Ball) {
	b.Pos.Add(Vector{b.Vel.X * dt, b.Vel.Y * dt})
}

// Circle-circle collision detection
func circlesCollided(b1, b2 *Ball) bool {
	dx := b2.Pos.X - b1.Pos.X
	dy := b2.Pos.Y - b1.Pos.Y
	dist := math.Sqrt(dx*dx + dy*dy)
	return dist < (b1.Radius + b2.Radius)
}

// Circle-circle collision response
func bounceBalls(b1, b2 *Ball) {
	normal := Vector{b2.Pos.X - b1.Pos.X, b2.Pos.Y - b1.Pos.Y}
	dist := normal.Length()
	if dist == 0 {
		return
	}
	n := normal.Normalized()

	// relative velocity
	rv := Vector{b2.Vel.X - b1.Vel.X, b2.Vel.Y - b1.Vel.Y}
	velAlongNormal := rv.X*n.X + rv.Y*n.Y

	if velAlongNormal > 0 {
		return
	}

	impulse := -(1 + e) * velAlongNormal
	impulse /= (1/b1.Mass + 1/b2.Mass)

	impulseVec := Vector{n.X * impulse, n.Y * impulse}
	b1.Vel.X -= (impulseVec.X / b1.Mass)
	b1.Vel.Y -= (impulseVec.Y / b1.Mass)
	b2.Vel.X += (impulseVec.X / b2.Mass)
	b2.Vel.Y += (impulseVec.Y / b2.Mass)

	// positional correction (prevent sinking)
	penetration := (b1.Radius + b2.Radius) - dist
	correction := Vector{n.X * penetration / 2, n.Y * penetration / 2}
	b1.Pos.X -= correction.X
	b1.Pos.Y -= correction.Y
	b2.Pos.X += correction.X
	b2.Pos.Y += correction.Y
}

// Wall collision. This needs to be slightly more robust to handle
// the boundary *and* the internal structure.
func bounceWall(b *Ball, w Wall) {
	// AABB (Axis-Aligned Bounding Box) collision check

	// Check top edge of the wall (e.g., floor)
	if b.Pos.Y+b.Radius > w.Y && b.Pos.Y+b.Radius < w.Y+w.H &&
		b.Pos.X > w.X && b.Pos.X < w.X+w.W && b.Vel.Y > 0 {
		b.Pos.Y = w.Y - b.Radius
		b.Vel.Y *= -e
		return
	}
	// Check bottom edge of the wall (e.g., ceiling)
	if b.Pos.Y-b.Radius < w.Y+w.H && b.Pos.Y-b.Radius > w.Y &&
		b.Pos.X > w.X && b.Pos.X < w.X+w.W && b.Vel.Y < 0 {
		b.Pos.Y = w.Y + w.H + b.Radius
		b.Vel.Y *= -e
		return
	}
	// Check left edge of the wall
	if b.Pos.X+b.Radius > w.X && b.Pos.X+b.Radius < w.X+w.W &&
		b.Pos.Y > w.Y && b.Pos.Y < w.Y+w.H && b.Vel.X > 0 {
		b.Pos.X = w.X - b.Radius
		b.Vel.X *= -e
		return
	}
	// Check right edge of the wall
	if b.Pos.X-b.Radius < w.X+w.W && b.Pos.X-b.Radius > w.X &&
		b.Pos.Y > w.Y && b.Pos.Y < w.Y+w.H && b.Vel.X < 0 {
		b.Pos.X = w.X + w.W + b.Radius
		b.Vel.X *= -e
		return
	}
}

// getColorBySpeed generates a color based on the ball's speed.
// Fast balls are Red (high kinetic energy), slow balls are Blue/Purple.
func getColorBySpeed(b *Ball) color.RGBA {
	maxSpeedSq := 500.0 // Max speed squared for mapping (adjustable)
	speedSq := math.Min(b.Vel.LengthSq(), maxSpeedSq)

	// Normalize speed (0.0 to 1.0)
	ratio := speedSq / maxSpeedSq

	// Map ratio to colors: Blue (0) -> Green/Yellow (0.5) -> Red (1)
	r := uint8(math.Min(ratio*2*255, 255))
	g := uint8(math.Min((1-math.Abs(ratio-0.5))*2*255, 255))
	bVal := uint8(math.Min((1-ratio)*2*255, 255))

	return color.RGBA{R: r, G: g, B: bVal, A: 255}
}

// ============================
// Ebiten Game Loop
// ============================

type Game struct{}

func (g *Game) Update() error {
	// 1. Handle user input
	g.handleInput()

	// 2. Physics simulation step
	for _, b := range balls {
		applyForce(b, gravity)
		updatePosition(b)
		b.Color = getColorBySpeed(b) // Update color based on velocity
	}

	// 3. Handle ball-wall collisions (boundaries and internal structures)
	for _, b := range balls {
		for _, w := range walls {
			bounceWall(b, w)
		}
	}

	// 4. Handle ball-ball collisions
	for i := 0; i < len(balls); i++ {
		for j := i + 1; j < len(balls); j++ {
			if circlesCollided(balls[i], balls[j]) {
				bounceBalls(balls[i], balls[j])
			}
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw the background
	screen.Fill(color.RGBA{20, 20, 40, 255}) // Dark blue background

	// Draw the walls (boundaries and internal)
	for _, w := range walls {
		// Use ebitenutil.DrawRect for simple drawing of walls
		ebitenutil.DrawRect(screen, w.X, w.Y, w.W, w.H, w.Color)
	}

	// Draw the balls
	for _, b := range balls {
		// Use ebitenutil.DrawCircle for the balls (easy to use)
		ebitenutil.DrawCircle(screen, b.Pos.X, b.Pos.Y, b.Radius, b.Color)
	}

	// Draw info text
	ebitenutil.DebugPrint(screen, "Balls: %d | Click/Tap to add ball")
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenW, screenH
}

// handleInput spawns a new ball at the mouse/touch position.
func (g *Game) handleInput() {
	spawn := false
	var x, y float64

	// Check mouse click
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		x, y = float64(mx), float64(my)
		spawn = true
	}

	// Check touch tap (for mobile compatibility)
	if len(inpututil.AppendJustPressedTouchIDs(nil)) > 0 {
		tid := inpututil.AppendJustPressedTouchIDs(nil)[0]
		tx, ty := ebiten.TouchPosition(tid)
		x, y = float64(tx), float64(ty)
		spawn = true
	}

	if spawn {
		// Ensure the new ball is within boundaries
		x = math.Max(BallRadius, math.Min(x, float64(screenW)-BallRadius))
		y = math.Max(BallRadius, math.Min(y, float64(screenH)-BallRadius))

		newBall := &Ball{
			Pos:    Vector{X: x, Y: y},
			Vel:    Vector{X: float64(rand.IntN(500)-250) / 100.0, Y: float64(rand.IntN(500)-250) / 100.0},
			Radius: 10,
			Mass:   1.0,
			Color:  color.RGBA{255, 255, 255, 255}, // Start white
		}
		balls = append(balls, newBall)
	}
}

// ============================
// Initialization
// ============================

const BallRadius = 10.0

func initGame(n int) {
	balls = make([]*Ball, 0, n)

	// Create initial balls
	for i := 0; i < n; i++ {
		b := &Ball{
			Pos:    Vector{float64(rand.IntN(screenW-40) + 20), float64(rand.IntN(screenH/4) + 20)},
			Vel:    Vector{float64(rand.IntN(10) - 5), float64(rand.IntN(10) - 5)},
			Radius: BallRadius,
			Mass:   1.0,
			Color:  color.RGBA{255, 255, 255, 255},
		}
		balls = append(balls, b)
	}

	// Define Walls
	wallColor := color.RGBA{100, 100, 100, 255}
	wallThickness := 20.0

	// 1. Boundary Walls
	walls = []Wall{
		// top
		{X: 0, Y: 0, W: float64(screenW), H: wallThickness, Color: wallColor},
		// bottom
		{X: 0, Y: float64(screenH) - wallThickness, W: float64(screenW), H: wallThickness, Color: wallColor},
		// left
		{X: 0, Y: 0, W: wallThickness, H: float64(screenH), Color: wallColor},
		// right
		{X: float64(screenW) - wallThickness, Y: 0, W: wallThickness, H: float64(screenH), Color: wallColor},

		// 2. Internal Obstacle (A Static Shelf/Ramp)
		{X: 100, Y: 650, W: 350, H: 30, Color: color.RGBA{200, 150, 0, 255}}, // Gold-colored shelf
		{X: 450, Y: 500, W: 50, H: 180, Color: color.RGBA{200, 150, 0, 255}}, // Pillar
	}
}

func main() {
	initGame(20) // Start with 20 balls
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("Kinetic Energy Visualizer")
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
