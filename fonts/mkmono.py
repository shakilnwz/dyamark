# Makes fonts/DyamarkMono-Regular.ttf from Liberation Mono: a blank glyph for
# the tab character, four spaces wide, and a name without the Reserved Font
# Names, as the OFL asks of a modified version.
import sys
from fontTools.ttLib import TTFont
from fontTools.ttLib.tables._g_l_y_f import Glyph

src, dst = sys.argv[1], sys.argv[2]
f = TTFont(src)
order = f.getGlyphOrder() + ['tab']
f['glyf'].glyphs['tab'] = Glyph()
f.setGlyphOrder(order)
f['glyf'].glyphOrder = order
f['hmtx']['tab'] = (4 * f['hmtx']['space'][0], 0)
for t in f['cmap'].tables:
    if t.isUnicode():
        t.cmap[0x09] = 'tab'
# Tables with an entry per glyph that tools rebuild; they're optional.
for tag in ('hdmx', 'LTSH', 'VDMX'):
    if tag in f:
        del f[tag]
for rec in f['name'].names:
    if rec.nameID in (1, 3, 4, 6, 16, 17, 18, 21, 22):
        s = rec.toUnicode().replace('Liberation Mono', 'Dyamark Mono').replace('LiberationMono', 'DyamarkMono')
        rec.string = s
f.save(dst)
g = TTFont(dst)
print(g['name'].getDebugName(1), g['name'].getDebugName(6), g['hmtx']['tab'], g.getBestCmap()[9], g['maxp'].numGlyphs)
