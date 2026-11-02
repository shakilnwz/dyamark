package dyamark.jetbrains

import com.intellij.ide.ui.LafManager
import com.intellij.ide.ui.LafManagerListener
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.colors.CodeInsightColors
import com.intellij.openapi.editor.colors.ColorKey
import com.intellij.openapi.editor.colors.EditorColorsListener
import com.intellij.openapi.editor.colors.EditorColorsManager
import com.intellij.openapi.editor.colors.EditorColorsScheme
import com.intellij.ui.JBColor
import java.awt.Color
import java.util.concurrent.CopyOnWriteArrayList

private val LOG = logger<ThemeBridge>()

/**
 * Tracks open [DyamarkPanel]s and keeps their preview palettes in sync with
 * the IDE's editor color scheme and Look & Feel live.
 */
object ThemeBridge {
    private val panels = CopyOnWriteArrayList<DyamarkPanel>()

    fun register(panel: DyamarkPanel) {
        panels.add(panel)
        panel.setTheme(currentThemeJson())
    }

    fun unregister(panel: DyamarkPanel) {
        panels.remove(panel)
    }

    fun updateAll() {
        val json = currentThemeJson()
        LOG.info("dyamark: editor scheme changed; updating ${panels.size} open preview(s)")
        for (panel in panels) {
            panel.setTheme(json)
        }
    }

    fun currentThemeJson(): String {
        val scheme = EditorColorsManager.getInstance().globalScheme
        return themeJson(scheme)
    }

    fun themeJson(scheme: EditorColorsScheme): String {
        val bg = scheme.defaultBackground
        val fg = scheme.defaultForeground
        val accent = scheme.getAttributes(CodeInsightColors.HYPERLINK_ATTRIBUTES)?.foregroundColor
            ?: scheme.getColor(ColorKey.find("HYPERLINK_COLOR"))
            ?: JBColor.namedColor("Link.activeForeground", JBColor(0x3574F0, 0x548AF7))
        val error = scheme.getAttributes(CodeInsightColors.ERRORS_ATTRIBUTES)?.effectColor
            ?: scheme.getAttributes(CodeInsightColors.ERRORS_ATTRIBUTES)?.errorStripeColor
            ?: JBColor.namedColor("Component.errorFocusColor", JBColor(0xDB5860, 0xE55765))

        return buildString {
            append('{')
            append("\"background\":").append(jsString(bg.toHex())).append(',')
            append("\"foreground\":").append(jsString(fg.toHex())).append(',')
            append("\"accent\":").append(jsString(accent.toHex())).append(',')
            append("\"error\":").append(jsString(error.toHex()))
            append('}')
        }
    }

    private fun Color.toHex(): String = String.format("#%02x%02x%02x", red, green, blue)
}

class DyamarkEditorColorsListener : EditorColorsListener {
    override fun globalSchemeChange(scheme: EditorColorsScheme?) {
        ThemeBridge.updateAll()
    }
}

class DyamarkLafListener : LafManagerListener {
    override fun lookAndFeelChanged(source: LafManager) {
        ThemeBridge.updateAll()
    }
}
