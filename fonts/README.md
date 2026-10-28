# Bundled fonts

Liberation Sans regular, bold, italic and bold italic, unchanged, and
Dyamark Mono, all under the SIL Open Font License in `LICENSE.liberation`.

Dyamark Mono is Liberation Mono 2.1.5 with one glyph added: a blank one for
the tab character, four spaces wide. Without it a tab shows as a box, in
code blocks and in the editor. The OFL doesn't let a changed font keep its
Reserved Font Names, so it's renamed. `mkmono.py` makes it, with fontTools:

    python mkmono.py /usr/share/fonts/liberation/LiberationMono-Regular.ttf DyamarkMono-Regular.ttf
