package render

import (
	"os"

	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/muesli/termenv"
)

type termenvPalette struct {
	out *termenv.Output
}

func PaletteFromEnv() Palette {
	t := term.FromEnv()
	if !t.IsColorEnabled() {
		return PlainPalette{}
	}
	return termenvPalette{out: termenv.NewOutput(os.Stdout)}
}

func (p termenvPalette) color(s, ansi string) string {
	return p.out.String(s).Foreground(p.out.Color(ansi)).String()
}

func (p termenvPalette) Green(s string) string  { return p.color(s, "2") }
func (p termenvPalette) Red(s string) string    { return p.color(s, "1") }
func (p termenvPalette) Yellow(s string) string { return p.color(s, "3") }
func (p termenvPalette) Cyan(s string) string   { return p.color(s, "6") }
func (p termenvPalette) Gray(s string) string   { return p.color(s, "8") }
func (p termenvPalette) Bold(s string) string {
	return p.out.String(s).Bold().String()
}
