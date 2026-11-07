package dyamark.jetbrains

import com.intellij.openapi.application.ApplicationInfo
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.jcef.JBCefApp
import org.intellij.plugins.markdown.settings.MarkdownSettings
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanel
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanelProvider

/**
 * Offers dyamark as a Markdown preview, beside the IDE's JCEF and Compose
 * ones. It keeps the default source text preprocessor, which hands the
 * panel the document's raw Markdown rather than HTML.
 */
class DyamarkPanelProvider : MarkdownHtmlPanelProvider() {
    override fun createHtmlPanel(): MarkdownHtmlPanel = DyamarkPanel(null, null)

    override fun createHtmlPanel(project: Project, virtualFile: VirtualFile): MarkdownHtmlPanel =
        DyamarkPanel(project, virtualFile)

    override fun isAvailable(): AvailabilityInfo =
        if (JBCefApp.isSupported()) AvailabilityInfo.AVAILABLE else AvailabilityInfo.UNAVAILABLE

    override fun getProviderInfo(): ProviderInfo = INFO

    companion object {
        val INFO = ProviderInfo("dyamark", DyamarkPanelProvider::class.java.name)
    }
}

/**
 * The Markdown settings: the application's since build 263, the project's
 * before. This asks for the service where it lives, as neither
 * [MarkdownSettings.getInstance] suits every build: the one without a project
 * isn't in 262, and the one with a project is deprecated from 263.
 */
internal fun markdownSettings(project: Project): MarkdownSettings =
    if (ApplicationInfo.getInstance().build.baselineVersion >= 263) {
        ApplicationManager.getApplication().getService(MarkdownSettings::class.java)
    } else {
        project.getService(MarkdownSettings::class.java)
    }
