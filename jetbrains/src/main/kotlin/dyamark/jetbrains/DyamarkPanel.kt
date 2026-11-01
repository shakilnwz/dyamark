package dyamark.jetbrains

import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.handler.CefLoadHandler
import org.cef.handler.CefLoadHandlerAdapter
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanel
import java.nio.file.Path
import javax.swing.JComponent

private val LOG = logger<DyamarkPanel>()

/**
 * One Markdown preview, drawn by dyamark's WebAssembly build in a JCEF
 * browser. The Markdown plugin hands it the raw Markdown through [setHtml].
 */
class DyamarkPanel(private val project: Project?, private val file: VirtualFile?) : MarkdownHtmlPanel {
    private val browser = JBCefBrowser.createBuilder()
        .setEnableOpenDevToolsMenuItem(true)
        .build()
    private val query = JBCefJSQuery.create(browser as JBCefBrowserBase)

    private val lock = Any()
    private var ready = false
    private var pending: String? = null

    init {
        Disposer.register(this, browser)
        Disposer.register(this, query)
        query.addHandler { message ->
            onMessage(message)
            null
        }
        browser.jbCefClient.addRequestHandler(WebResources, browser.cefBrowser)
        browser.jbCefClient.addLoadHandler(object : CefLoadHandlerAdapter() {
            override fun onLoadError(
                cef: CefBrowser?, frame: CefFrame?, errorCode: CefLoadHandler.ErrorCode?, errorText: String?, failedUrl: String?,
            ) {
                LOG.warn("dyamark ${file?.name}: load error $errorCode $errorText $failedUrl")
            }

            override fun onLoadEnd(cef: CefBrowser, frame: CefFrame, httpStatusCode: Int) {
                LOG.info("dyamark ${file?.name}: load end ${frame.url} $httpStatusCode main=${frame.isMain}")
                if (!frame.isMain) return
                val js = "window.dyamarkHostPost = function(m) { ${query.inject("m")} }; window.dyamarkHostReady();"
                cef.executeJavaScript(js, WebResources.INDEX, 0)
            }
        }, browser.cefBrowser)
        browser.loadURL(WebResources.INDEX)
        Spike.panelCreated(this)
    }

    override fun getComponent(): JComponent = browser.component

    internal val cefBrowser get() = browser.cefBrowser

    override fun getProject(): Project? = project

    override fun getVirtualFile(): VirtualFile? = file

    override fun setHtml(html: String, initialScrollOffset: Int, document: VirtualFile?) {
        push(html)
    }

    override fun setHtml(html: String, initialScrollOffset: Int, documentPath: Path?) {
        push(html)
    }

    private fun push(html: String) {
        Spike.sourceSent(this)
        synchronized(lock) {
            if (!ready) {
                pending = html
                return
            }
        }
        setSource(html)
    }

    override fun reloadWithOffset(offset: Int) {}

    override fun scrollToMarkdownSrcOffset(offset: Int, smooth: Boolean) {}

    override fun addScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {}

    override fun removeScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {}

    override fun dispose() {}

    private fun setSource(text: String) {
        browser.cefBrowser.executeJavaScript("dyamark.setSource(${jsString(text)})", WebResources.INDEX, 0)
    }

    private fun onMessage(message: String) {
        LOG.info("dyamark ${file?.name}: $message")
        if (message.startsWith("ready")) {
            val text = synchronized(lock) {
                ready = true
                pending.also { pending = null }
            }
            if (text != null) setSource(text)
        }
        Spike.message(this, message)
    }
}

/** Quotes [s] as a JavaScript string literal. */
internal fun jsString(s: String): String = buildString(s.length + 16) {
    append('"')
    for (c in s) {
        when (c) {
            '"' -> append("\\\"")
            '\\' -> append("\\\\")
            '\n' -> append("\\n")
            '\r' -> append("\\r")
            '\t' -> append("\\t")
            ' ' -> append("\\u2028")
            ' ' -> append("\\u2029")
            '<' -> append("\\u003c")
            else -> if (c < ' ') append("\\u%04x".format(c.code)) else append(c)
        }
    }
    append('"')
}
