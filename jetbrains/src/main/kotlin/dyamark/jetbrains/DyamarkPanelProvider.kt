package dyamark.jetbrains

import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.jcef.JBCefApp
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
