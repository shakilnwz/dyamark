package dyamark.jetbrains

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class DyamarkPanelTest {
    @Test
    fun testComputeOffsetToLine() {
        val text = "Line 1\nLine 2\n\nLine 4\n"
        // Offsets:
        // Line 1: 0..6 ('\n' is at 6)
        // Line 2: 7..13 ('\n' is at 13)
        // Line 3: 14 ('\n' is at 14)
        // Line 4: 15..21 ('\n' is at 21)
        // Line 5: 22 (empty trailing)

        assertEquals(1, computeOffsetToLine("", 0))
        assertEquals(1, computeOffsetToLine(text, -5))
        assertEquals(1, computeOffsetToLine(text, 0))
        assertEquals(1, computeOffsetToLine(text, 5))
        assertEquals(1, computeOffsetToLine(text, 6)) // on the first '\n'
        assertEquals(2, computeOffsetToLine(text, 7)) // first char of Line 2
        assertEquals(2, computeOffsetToLine(text, 13))
        assertEquals(3, computeOffsetToLine(text, 14)) // empty Line 3
        assertEquals(4, computeOffsetToLine(text, 15)) // Line 4
        assertEquals(4, computeOffsetToLine(text, 21))
        assertEquals(5, computeOffsetToLine(text, 22))
        assertEquals(5, computeOffsetToLine(text, 100))
    }

    @Test
    fun testComputeLineToOffset() {
        val text = "Line 1\nLine 2\n\nLine 4\n"

        assertEquals(0, computeLineToOffset("", 1))
        assertEquals(0, computeLineToOffset(text, 0))
        assertEquals(0, computeLineToOffset(text, -1))
        assertEquals(0, computeLineToOffset(text, 1))
        assertEquals(7, computeLineToOffset(text, 2))
        assertEquals(14, computeLineToOffset(text, 3))
        assertEquals(15, computeLineToOffset(text, 4))
        assertEquals(22, computeLineToOffset(text, 5))
        assertEquals(text.length, computeLineToOffset(text, 10))
    }

    @Test
    fun testOffsetAndLineRoundtrip() {
        val text = "# Title\n\nParagraph 1\n\n```mermaid\nsequenceDiagram\n```\n"
        val lines = text.split('\n')
        for (i in 1..lines.size) {
            val offset = computeLineToOffset(text, i)
            val line = computeOffsetToLine(text, offset)
            assertEquals(i, line, "Line $i roundtrip through offset $offset")
        }
    }

    @Test
    fun testIsWebLink() {
        assertTrue(isWebLink("http://example.com"))
        assertTrue(isWebLink("https://example.com/page"))
        assertTrue(isWebLink("HTTPS://EXAMPLE.COM"))
        assertTrue(isWebLink("mailto:test@example.com"))

        assertFalse(isWebLink("#header"))
        assertFalse(isWebLink("file.md"))
        assertFalse(isWebLink("./sub/doc.md"))
        assertFalse(isWebLink("../parent.md"))
        assertFalse(isWebLink("file:///path/to/doc.md"))
    }

    @Test
    fun testJsStringEscaping() {
        assertEquals("\"hello\"", jsString("hello"))
        assertEquals("\"line 1\\nline 2\"", jsString("line 1\nline 2"))
        assertEquals("\"tab\\there\"", jsString("tab\there"))
        assertEquals("\"with \\\"quotes\\\" and \\\\ backslash\"", jsString("with \"quotes\" and \\ backslash"))
        assertEquals("\"tag \\u003cscript>\"", jsString("tag <script>"))
    }

    @Test
    fun testIsMarkdownFile() {
        assertTrue(isMarkdownFile("README.md"))
        assertTrue(isMarkdownFile("doc.markdown"))
        assertTrue(isMarkdownFile("notes.mdown"))
        assertTrue(isMarkdownFile("file.mkdn"))
        assertTrue(isMarkdownFile("UPPER.MD"))
        assertTrue(isMarkdownFile("PATH/TO/DOC.MARKDOWN"))

        assertFalse(isMarkdownFile("code.go"))
        assertFalse(isMarkdownFile("notes.txt"))
        assertFalse(isMarkdownFile("script.sh"))
        assertFalse(isMarkdownFile("markdown"))
        assertFalse(isMarkdownFile(""))
    }

    @Test
    fun testDyamarkNotificationShouldNotify() {
        val jcef = org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanelProvider.ProviderInfo("JCEF Browser", "com.intellij.markdown.jcef.preview.JCEFHtmlPanelProvider")

        // Never shown, and currently JCEF or null -> should notify
        assertTrue(DyamarkNotification.shouldNotify(isShown = false, currentProvider = null))
        assertTrue(DyamarkNotification.shouldNotify(isShown = false, currentProvider = jcef))

        // Already shown once -> should not notify
        assertFalse(DyamarkNotification.shouldNotify(isShown = true, currentProvider = null))
        assertFalse(DyamarkNotification.shouldNotify(isShown = true, currentProvider = jcef))
        assertFalse(DyamarkNotification.shouldNotify(isShown = true, currentProvider = DyamarkPanelProvider.INFO))

        // Never shown, but already using Dyamark -> should not notify
        assertFalse(DyamarkNotification.shouldNotify(isShown = false, currentProvider = DyamarkPanelProvider.INFO))
    }

    @Test
    fun testProviderInfo() {
        assertEquals("dyamark", DyamarkPanelProvider.INFO.name)
        assertEquals(DyamarkPanelProvider::class.java.name, DyamarkPanelProvider.INFO.className)
    }

    @Test
    fun testJsLineHasAPointInAnyLocale() {
        val before = java.util.Locale.getDefault()
        try {
            java.util.Locale.setDefault(java.util.Locale.GERMANY)
            assertEquals("31.420", jsLine(31.42))
            assertEquals("1.000", jsLine(1.0))
        } finally {
            java.util.Locale.setDefault(before)
        }
    }
}
