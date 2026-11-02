package dyamark.jetbrains

import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.wm.IdeFocusManager
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.handler.CefLoadHandler
import org.cef.handler.CefLoadHandlerAdapter
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanel
import java.awt.event.MouseAdapter
import java.awt.event.MouseEvent
import java.nio.file.Path
import java.util.concurrent.CopyOnWriteArrayList
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
    private var currentSource: String? = null
    private val scrollListeners = CopyOnWriteArrayList<MarkdownHtmlPanel.ScrollListener>()
    private var isPanelDisposed = false

    init {
        Disposer.register(this, browser)
        Disposer.register(this, query)

        browser.component.putClientProperty(PANEL_CLIENT_PROPERTY, this)
        (browser.cefBrowser.uiComponent as? JComponent)?.putClientProperty(PANEL_CLIENT_PROPERTY, this)

        setupFocusOnMousePress()

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
                if (!frame.isMain || isPanelDisposed || browser.isDisposed) return
                synchronized(lock) {
                    ready = false
                }
                val js = "window.dyamarkHostPost = function(m) { ${query.inject("m")} }; window.dyamarkHostReady();"
                cef.executeJavaScript(js, WebResources.INDEX, 0)
            }
        }, browser.cefBrowser)
        browser.loadURL(WebResources.INDEX)
        Spike.panelCreated(this)
    }

    private fun setupFocusOnMousePress() {
        val listener = object : MouseAdapter() {
            override fun mousePressed(e: MouseEvent) {
                requestFocus()
            }
        }
        browser.component.addMouseListener(listener)
        browser.cefBrowser.uiComponent.addMouseListener(listener)
    }

    fun requestFocus() {
        val ui = browser.cefBrowser.uiComponent
        if (project != null && !project.isDisposed) {
            IdeFocusManager.getInstance(project).requestFocus(ui, true)
        } else {
            IdeFocusManager.getGlobalInstance().requestFocus(ui, true)
        }
        browser.cefBrowser.setFocus(true)
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
        val sendNow: Boolean
        synchronized(lock) {
            currentSource = html
            sendNow = ready
        }
        if (sendNow) {
            setSource(html)
        }
    }

    fun zoomIn() {
        executeJs("if (globalThis.dyamark?.zoomIn) dyamark.zoomIn();")
    }

    fun zoomOut() {
        executeJs("if (globalThis.dyamark?.zoomOut) dyamark.zoomOut();")
    }

    fun resetZoom() {
        executeJs("if (globalThis.dyamark?.resetZoom) dyamark.resetZoom();")
    }

    private fun executeJs(code: String) {
        if (!browser.isDisposed && !isPanelDisposed) {
            browser.cefBrowser.executeJavaScript(code, WebResources.INDEX, 0)
        }
    }

    override fun reloadWithOffset(offset: Int) {
        if (!browser.isDisposed && !isPanelDisposed) {
            browser.cefBrowser.reload()
        }
    }

    @Deprecated("Deprecated in MarkdownHtmlPanel", ReplaceWith(""))
    @Suppress("OVERRIDE_DEPRECATION", "DEPRECATION")
    override fun scrollToMarkdownSrcOffset(offset: Int, smooth: Boolean) {}

    override fun addScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {
        scrollListeners.add(listener)
    }

    override fun removeScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {
        scrollListeners.remove(listener)
    }

    override fun dispose() {
        LOG.info("dyamark ${file?.name}: disposed")
        isPanelDisposed = true
        if (!query.isDisposed) {
            Disposer.dispose(query)
        }
        if (!browser.isDisposed) {
            Disposer.dispose(browser)
        }
    }

    private fun setSource(text: String) {
        executeJs("dyamark.setSource(${jsString(text)})")
    }

    private fun onMessage(message: String) {
        if (isPanelDisposed || browser.isDisposed) return
        LOG.info("dyamark ${file?.name}: $message")
        when {
            message.startsWith("ready") -> {
                val text = synchronized(lock) {
                    ready = true
                    currentSource
                }
                if (text != null) setSource(text)
            }
            message.startsWith("error ") -> {
                LOG.warn("dyamark ${file?.name}: ${message.removePrefix("error ")}")
            }
            message.startsWith("log ") -> {
                LOG.info("dyamark ${file?.name}: ${message.removePrefix("log ")}")
            }
        }
        Spike.message(this, message)
    }

    companion object {
        const val PANEL_CLIENT_PROPERTY = "dyamark.panel"
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
