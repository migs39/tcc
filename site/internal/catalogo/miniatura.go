package catalogo

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // registra o decodificador PNG
	"math"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registra o decodificador WebP
)

// Miniatura do pacote do catálogo (D-006): todas com as mesmas dimensões e o
// mesmo formato, para o pacote não crescer com o tamanho das capas originais.
const (
	LarguraMiniatura   = 200
	AlturaMiniatura    = 300
	qualidadeMiniatura = 80
	// Limite de pixels da imagem de entrada, contra imagens que expandem para
	// muita memória ao decodificar.
	maxPixelsCapa = 40_000_000
)

var ErrCapaIlegivel = errors.New("não foi possível ler a imagem da capa")

// GerarMiniatura encaixa a capa num quadro fixo de 200×300 (fundo branco,
// proporção preservada) e grava em JPEG. A reencodificação também descarta
// metadados embutidos na imagem original.
func GerarMiniatura(dados []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(dados))
	if err != nil {
		return nil, ErrCapaIlegivel
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixelsCapa {
		return nil, ErrCapaTamanho
	}
	orig, _, err := image.Decode(bytes.NewReader(dados))
	if err != nil {
		return nil, ErrCapaIlegivel
	}

	quadro := image.NewRGBA(image.Rect(0, 0, LarguraMiniatura, AlturaMiniatura))
	xdraw.Draw(quadro, quadro.Bounds(), &image.Uniform{C: color.White}, image.Point{}, xdraw.Src)

	b := orig.Bounds()
	escala := math.Min(float64(LarguraMiniatura)/float64(b.Dx()), float64(AlturaMiniatura)/float64(b.Dy()))
	w := max(1, int(math.Round(float64(b.Dx())*escala)))
	h := max(1, int(math.Round(float64(b.Dy())*escala)))
	x0, y0 := (LarguraMiniatura-w)/2, (AlturaMiniatura-h)/2
	xdraw.CatmullRom.Scale(quadro, image.Rect(x0, y0, x0+w, y0+h), orig, b, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, quadro, &jpeg.Options{Quality: qualidadeMiniatura}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
