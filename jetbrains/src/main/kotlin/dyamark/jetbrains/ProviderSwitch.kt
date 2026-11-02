package dyamark.jetbrains

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import org.intellij.plugins.markdown.settings.MarkdownSettings

private val LOG = logger<DyamarkProviderSwitchListener>()

/**
 * Makes choosing another Markdown preview swap the open previews, from the
 * Settings dialog and from the notification alike.
 *
 * In 2026.2, a preview editor told that the settings changed rebuilds its
 * panel only when the provider it used last is the one chosen now, and
 * then notes the chosen one. So the change that switches providers keeps
 * every open preview as it was, and only a later change of any Markdown
 * setting swaps them. This tells the listeners once more, with nothing
 * changed.
 */
class DyamarkProviderSwitchListener(private val project: Project) : MarkdownSettings.ChangeListener {
    private var before: String? = null

    override fun beforeSettingsChanged(settings: MarkdownSettings) {
        before = settings.previewPanelProviderInfo.className
    }

    override fun settingsChanged(settings: MarkdownSettings) {
        val after = settings.previewPanelProviderInfo.className
        val switched = before != null && before != after
        before = null
        if (!switched) return
        LOG.info("dyamark: the Markdown preview is now $after; swapping the open previews")
        ApplicationManager.getApplication().invokeLater({ settings.update { } }, project.disposed)
    }
}
