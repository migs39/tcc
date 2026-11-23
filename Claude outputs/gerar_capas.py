"""Gera capas tipográficas para os livros da demonstração.

Uma capa por livro, 600x900 px (proporção 2:3), com título, autor, ano e
categoria nas fontes do tema (Space Grotesk e JetBrains Mono, OFL, em
protocolo/tema/fontes). Sem imagens de terceiros, portanto sem questão de
direitos autorais. O fundo de cada capa vem de uma paleta fixa, escolhida
pelo id do livro, para que os livros se distingam na grade.

Uso: python -I gerar_capas.py <livros.tsv> <pasta de saída>
     livros.tsv: id<TAB>título<TAB>autor<TAB>ano<TAB>categoria, uma linha por livro.
Depois, envie cada <id>.jpg pela página do livro no admin (RF02).
"""
import os
import sys

from PIL import Image, ImageDraw, ImageFont

RAIZ = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FONTES = os.path.join(RAIZ, 'protocolo', 'tema', 'fontes')
L, A = 600, 900
MARGEM = 56

# (fundo, tom do motivo, destaque): tons escuros que combinam com o tema.
PALETA = [
    ('#10261F', '#183A2F', '#39D98A'),  # verde
    ('#16203A', '#202E52', '#7FA8FF'),  # azul
    ('#2E1621', '#43202F', '#F08FB0'),  # vinho
    ('#2A2414', '#3C331C', '#E8C547'),  # ocre
    ('#132A33', '#1C3B47', '#5FD4E8'),  # petróleo
    ('#2B1C14', '#3E281C', '#F0A36B'),  # terracota
]
CLARO = '#F2F1EC'
SUAVE = '#B9C2BE'


def fonte(nome, tam, peso):
    f = ImageFont.truetype(os.path.join(FONTES, nome), tam)
    f.set_variation_by_axes([peso])
    return f


def quebrar(texto, f, largura):
    """Quebra em linhas que cabem na largura, só nos espaços."""
    linhas, atual = [], ''
    for palavra in texto.split():
        teste = (atual + ' ' + palavra).strip()
        if f.getlength(teste) <= largura or not atual:
            atual = teste
        else:
            linhas.append(atual)
            atual = palavra
    if atual:
        linhas.append(atual)
    return linhas


def titulo_que_cabe(texto, largura, altura_max):
    """Maior tamanho de título (de 76 a 40 px) que cabe em até 5 linhas."""
    for tam in range(76, 39, -4):
        f = fonte('SpaceGrotesk.woff2', tam, 700)
        linhas = quebrar(texto, f, largura)
        if len(linhas) <= 5 and len(linhas) * tam * 1.12 <= altura_max:
            return f, tam, linhas
    return f, tam, linhas


def capa(livro_id, titulo, autor, ano, categoria):
    fundo, motivo, destaque = PALETA[livro_id % len(PALETA)]
    im = Image.new('RGB', (L, A), fundo)
    d = ImageDraw.Draw(im)

    # Motivo: arcos concêntricos no canto inferior direito, como um selo.
    cx, cy = L - 40, A - 120
    for r in range(60, 520, 34):
        d.ellipse((cx - r, cy - r, cx + r, cy + r), outline=motivo, width=2)

    mono = fonte('JetBrainsMono.woff2', 22, 500)
    mono_b = fonte('JetBrainsMono.woff2', 26, 700)

    # Topo: categoria e uma linha de destaque.
    d.text((MARGEM, MARGEM), categoria.upper(), font=mono, fill=destaque)
    d.line((MARGEM, MARGEM + 40, MARGEM + 72, MARGEM + 40), fill=destaque, width=4)

    # Título.
    f, tam, linhas = titulo_que_cabe(titulo, L - 2 * MARGEM, 420)
    y = 190
    for linha in linhas:
        d.text((MARGEM, y), linha, font=f, fill=CLARO)
        y += int(tam * 1.12)

    # Autor e ano.
    y += 28
    for linha in quebrar(autor, mono_b, L - 2 * MARGEM):
        d.text((MARGEM, y), linha, font=mono_b, fill=SUAVE)
        y += 36
    d.text((MARGEM, y + 4), str(ano), font=mono, fill=SUAVE)

    # Rodapé: marca da livraria.
    d.text((MARGEM, A - MARGEM - 24), 'LIVRARIA OT', font=mono, fill=destaque)
    return im


def main():
    entrada, saida = sys.argv[1], sys.argv[2]
    os.makedirs(saida, exist_ok=True)
    with open(entrada, encoding='utf-8') as arq:
        for linha in arq:
            if not linha.strip():
                continue
            livro_id, titulo, autor, ano, categoria = linha.rstrip('\n').split('\t')
            im = capa(int(livro_id), titulo, autor, ano, categoria)
            destino = os.path.join(saida, f'{livro_id}.jpg')
            im.save(destino, 'JPEG', quality=90)
            print(destino)


if __name__ == '__main__':
    main()
