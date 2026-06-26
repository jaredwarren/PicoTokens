package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"

	"math"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Stats holds the daily usage metrics for the AI providers
type Stats struct {
	GeminiCost        float64
	GeminiWeeklyCost  float64
	GeminiInputToken  int64
	GeminiOutputToken int64
	ClaudeCost        float64
	ClaudeWeeklyCost  float64
	ClaudeInputToken  int64
	ClaudeOutputToken int64
	ClaudeResetTime   string
	LastUpdated       time.Time
}

// Renderer handles loading fonts and drawing the dashboard image.
type Renderer struct {
	fontRegular font.Face
	fontBold    font.Face
	fontLarge   font.Face
}

// NewRenderer loads fonts and initializes font faces.
func NewRenderer(regPath, boldPath string) (*Renderer, error) {
	regBytes, err := os.ReadFile(regPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read regular font: %w", err)
	}
	boldBytes, err := os.ReadFile(boldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bold font: %w", err)
	}

	regFont, err := opentype.Parse(regBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse regular font: %w", err)
	}
	boldFont, err := opentype.Parse(boldBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bold font: %w", err)
	}

	// Create faces
	faceReg, err := opentype.NewFace(regFont, &opentype.FaceOptions{
		Size:    11,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}

	faceBold, err := opentype.NewFace(boldFont, &opentype.FaceOptions{
		Size:    13,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}

	faceLarge, err := opentype.NewFace(boldFont, &opentype.FaceOptions{
		Size:    20,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}

	return &Renderer{
		fontRegular: faceReg,
		fontBold:    faceBold,
		fontLarge:   faceLarge,
	}, nil
}

// DrawDashboard generates a 296x128 landscape image containing the stats.
func (r *Renderer) DrawDashboard(s Stats, dailyBudget float64) image.Image {
	// Create canvas (296x128)
	img := image.NewRGBA(image.Rect(0, 0, 296, 128))

	// 1. Fill background with Solid Black
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

	// 2. Draw Header
	r.drawRobotIcon(img, 15, 5, color.White)
	r.drawText(img, "CLAUDE CODE USAGE", 35, 17, r.fontBold, color.White)
	syncTime := fmt.Sprintf("SYNC: %s", s.LastUpdated.Format("15:04"))
	r.drawTextRight(img, syncTime, 281, 17, r.fontRegular, color.White)

	// Header line
	r.drawLine(img, 15, 24, 281, 24, color.White)

	// Calculate percentages
	sessionPct := 0.0
	if dailyBudget > 0 {
		sessionPct = (s.ClaudeCost / dailyBudget) * 100.0
	}
	if sessionPct > 100.0 {
		sessionPct = 100.0
	}

	weeklyPct := 0.0
	if dailyBudget > 0 {
		weeklyPct = (s.ClaudeWeeklyCost / (dailyBudget * 7.0)) * 100.0
	}
	if weeklyPct > 100.0 {
		weeklyPct = 100.0
	}

	// 3. Session Row
	r.drawText(img, "SESSION", 15, 43, r.fontBold, color.White)
	sessionDetail := fmt.Sprintf("%d%%", int(math.Round(sessionPct)))
	r.drawTextRight(img, sessionDetail, 281, 43, r.fontRegular, color.White)
	r.drawBar(img, s.ClaudeCost, dailyBudget, 15, 51, 266)

	// 4. Weekly Row
	r.drawText(img, "WEEKLY", 15, 78, r.fontBold, color.White)
	weeklyDetail := fmt.Sprintf("%d%%", int(math.Round(weeklyPct)))
	r.drawTextRight(img, weeklyDetail, 281, 78, r.fontRegular, color.White)
	r.drawBar(img, s.ClaudeWeeklyCost, dailyBudget*7.0, 15, 86, 266)

	// 5. Footer Row (Reset times / optional Tokens)
	resetStr := "--"
	if s.ClaudeResetTime != "" {
		resetStr = s.ClaudeResetTime
	}
	r.drawText(img, fmt.Sprintf("RESETS: %s", resetStr), 15, 118, r.fontRegular, color.White)

	if s.ClaudeInputToken > 0 || s.ClaudeOutputToken > 0 {
		inTextClaude := fmt.Sprintf("In:%s Out:%s", formatTokens(s.ClaudeInputToken), formatTokens(s.ClaudeOutputToken))
		r.drawTextRight(img, inTextClaude, 281, 118, r.fontRegular, color.White)
	}

	return img
}

// drawBar draws a wide horizontal progress bar with a clean outline and filled inner area.
func (r *Renderer) drawBar(img draw.Image, val, max float64, x, y, width int) {
	r.drawRect(img, x, y, x+width, y+8, color.White)

	ratio := 0.0
	if max > 0 {
		ratio = val / max
	}
	if ratio > 1.0 {
		ratio = 1.0
	} else if ratio < 0.0 {
		ratio = 0.0
	}

	fillWidth := int(math.Round(ratio * float64(width-4)))
	if fillWidth > 0 {
		r.fillRect(img, x+2, y+2, x+2+fillWidth, y+6, color.White)
	}
}

// Rotate90CW rotates a 296x128 image 90 degrees clockwise to 128x296.
func Rotate90CW(src image.Image) *image.Gray {
	bounds := src.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y           // 296, 128
	dst := image.NewGray(image.Rect(0, 0, h, w)) // 128, 296

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Clockwise rotation formula: (x, y) -> (h - 1 - y, x)
			c := src.At(x, y)
			dst.Set(h-1-y, x, c)
		}
	}
	return dst
}

// PackImage packs a rotated 128x296 image into a 1-bit per pixel format expected by Waveshare.
// Length is exactly (128/8) * 296 = 4736 bytes.
// 1 = White, 0 = Black.
func PackImage(img *image.Gray) []byte {
	bounds := img.Bounds()
	width := bounds.Max.X  // 128
	height := bounds.Max.Y // 296
	buf := make([]byte, (width/8)*height)

	idx := 0
	for y := 0; y < height; y++ {
		for xByte := 0; xByte < width/8; xByte++ {
			var b byte = 0
			for bit := 0; bit < 8; bit++ {
				x := xByte*8 + bit
				grayVal := img.GrayAt(x, y).Y
				// Thresholding: pixel value > 127 is White (1), else Black (0)
				var bitVal byte = 0
				if grayVal > 127 {
					bitVal = 1
				}
				// Waveshare e-paper bit layout is MSB-first
				b |= bitVal << (7 - bit)
			}
			buf[idx] = b
			idx++
		}
	}
	return buf
}

// Helpers for drawing text and shapes

func (r *Renderer) drawText(img draw.Image, text string, x, y int, face font.Face, col color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	d.DrawString(text)
}

func (r *Renderer) drawTextRight(img draw.Image, text string, xRight, y int, face font.Face, col color.Color) {
	d := &font.Drawer{Face: face}
	width := d.MeasureString(text).Round()
	r.drawText(img, text, xRight-width, y, face, col)
}

func (r *Renderer) drawLine(img draw.Image, x1, y1, x2, y2 int, col color.Color) {
	// Simple horizontal/vertical line drawing (our layout only needs straight orthogonal lines)
	if x1 == x2 {
		if y1 > y2 {
			y1, y2 = y2, y1
		}
		for y := y1; y <= y2; y++ {
			img.Set(x1, y, col)
		}
	} else if y1 == y2 {
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		for x := x1; x <= x2; x++ {
			img.Set(x, y1, col)
		}
	}
}

func (r *Renderer) drawRect(img draw.Image, x1, y1, x2, y2 int, col color.Color) {
	r.drawLine(img, x1, y1, x2, y1, col)
	r.drawLine(img, x1, y2, x2, y2, col)
	r.drawLine(img, x1, y1, x1, y2, col)
	r.drawLine(img, x2, y1, x2, y2, col)
}

func (r *Renderer) fillRect(img draw.Image, x1, y1, x2, y2 int, col color.Color) {
	for y := y1; y <= y2; y++ {
		r.drawLine(img, x1, y, x2, y, col)
	}
}

func formatTokens(tokens int64) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1000.0)
	}
	return fmt.Sprintf("%d", tokens)
}

// drawProgressBar draws a small progress bar with a label
func (r *Renderer) drawProgressBar(img draw.Image, label string, val, max float64, x, y, width int) {
	// Draw label
	r.drawText(img, label, x, y+6, r.fontRegular, color.White)

	// Draw bar box outline (height 6, from y to y+6, X from x+30 to x+30+width)
	barXStart := x + 30
	barXEnd := barXStart + width
	r.drawRect(img, barXStart, y, barXEnd, y+6, color.White)

	// Compute ratio
	ratio := 0.0
	if max > 0 {
		ratio = val / max
	}
	if ratio > 1.0 {
		ratio = 1.0
	} else if ratio < 0.0 {
		ratio = 0.0
	}

	fillWidth := int(math.Round(ratio * float64(width-4)))
	if fillWidth > 0 {
		r.fillRect(img, barXStart+2, y+2, barXStart+2+fillWidth, y+4, color.White)
	}
}

// drawRobotIcon draws a small 13x11 pixel-art robot face in the header
func (r *Renderer) drawRobotIcon(img draw.Image, x, y int, col color.Color) {
	// Head box (solid white)
	for hx := x + 2; hx <= x+10; hx++ {
		for hy := y + 2; hy <= y+10; hy++ {
			img.Set(hx, hy, col)
		}
	}
	// Ears
	for hy := y + 4; hy <= y+7; hy++ {
		img.Set(x, hy, col)
		img.Set(x+1, hy, col)
		img.Set(x+11, hy, col)
		img.Set(x+12, hy, col)
	}
	// Antenna
	img.Set(x+6, y, col)
	img.Set(x+6, y+1, col)

	// Eyes (black cutout)
	img.Set(x+4, y+5, color.Black)
	img.Set(x+8, y+5, color.Black)

	// Mouth (black cutout)
	for mx := x + 4; mx <= x+8; mx++ {
		img.Set(mx, y+8, color.Black)
	}
}
