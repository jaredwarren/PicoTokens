package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"math"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Stats holds the daily usage metrics for the AI providers
type Stats struct {
	OpenAICost        float64
	OpenAIInputToken  int64
	OpenAIOutputToken int64
	ClaudeCost        float64
	ClaudeInputToken  int64
	ClaudeOutputToken int64
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
	regBytes, err := ioutil.ReadFile(regPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read regular font: %w", err)
	}
	boldBytes, err := ioutil.ReadFile(boldPath)
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
		Size:    9,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}

	faceBold, err := opentype.NewFace(boldFont, &opentype.FaceOptions{
		Size:    10,
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
	r.drawText(img, "🤖 AI TOKENS", 10, 16, r.fontBold, color.White)
	syncTime := fmt.Sprintf("SYNC: %s", s.LastUpdated.Format("15:04"))
	r.drawTextRight(img, syncTime, 286, 16, r.fontRegular, color.White)

	// Header line
	r.drawLine(img, 10, 22, 286, 22, color.White)

	// 3. OpenAI Card (Left)
	r.drawText(img, "OPENAI", 15, 36, r.fontBold, color.White)
	openAICostStr := fmt.Sprintf("$%.2f", s.OpenAICost)
	r.drawText(img, openAICostStr, 15, 60, r.fontLarge, color.White)
	
	inTextOpenAI := fmt.Sprintf("In:  %s", formatTokens(s.OpenAIInputToken))
	outTextOpenAI := fmt.Sprintf("Out: %s", formatTokens(s.OpenAIOutputToken))
	r.drawText(img, inTextOpenAI, 15, 76, r.fontRegular, color.White)
	r.drawText(img, outTextOpenAI, 15, 88, r.fontRegular, color.White)

	// 4. Middle Divider (Dotted/Dashed effect)
	for y := 28; y < 96; y += 4 {
		img.Set(148, y, color.White)
	}

	// 5. Anthropic/Claude Card (Right)
	r.drawText(img, "CLAUDE", 160, 36, r.fontBold, color.White)
	claudeCostStr := fmt.Sprintf("$%.2f", s.ClaudeCost)
	r.drawText(img, claudeCostStr, 160, 60, r.fontLarge, color.White)

	inTextClaude := fmt.Sprintf("In:  %s", formatTokens(s.ClaudeInputToken))
	outTextClaude := fmt.Sprintf("Out: %s", formatTokens(s.ClaudeOutputToken))
	r.drawText(img, inTextClaude, 160, 76, r.fontRegular, color.White)
	r.drawText(img, outTextClaude, 160, 88, r.fontRegular, color.White)

	// 6. Footer (Progress Bar & Combined Cost)
	r.drawLine(img, 10, 98, 286, 98, color.White)

	totalCost := s.OpenAICost + s.ClaudeCost
	budgetInfo := fmt.Sprintf("SPEND: $%.2f / $%.2f", totalCost, dailyBudget)
	r.drawText(img, budgetInfo, 10, 112, r.fontRegular, color.White)

	// Progress bar container (x: 150 to 286, y: 104 to 110)
	barXStart := 160
	barXEnd := 286
	barYStart := 104
	barYEnd := 110
	barWidth := barXEnd - barXStart

	// Draw outer bar outline
	r.drawRect(img, barXStart, barYStart, barXEnd, barYEnd, color.White)

	// Fill progress bar according to spend ratio
	ratio := totalCost / dailyBudget
	if ratio > 1.0 {
		ratio = 1.0
	} else if ratio < 0.0 {
		ratio = 0.0
	}
	fillWidth := int(math.Round(ratio * float64(barWidth-4)))
	if fillWidth > 0 {
		r.fillRect(img, barXStart+2, barYStart+2, barXStart+2+fillWidth, barYEnd-2, color.White)
	}

	return img
}

// Rotate90CW rotates a 296x128 image 90 degrees clockwise to 128x296.
func Rotate90CW(src image.Image) *image.Gray {
	bounds := src.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y // 296, 128
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
