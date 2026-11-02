package dyamark.jetbrains

import com.intellij.ide.BrowserUtil
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.project.Project
import com.intellij.openapi.project.guessProjectDir
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.wm.IdeFocusManager
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.handler.CefLoadHandler
import org.cef.handler.CefLoadHandlerAdapter
import org.intellij.plugins.markdown.ui.preview.accessor.MarkdownLinkOpener
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
    private var currentFile: VirtualFile? = file
    private var currentPath: Path? = runCatching { file?.toNioPath() }.getOrNull()
    private var pendingScrollLine: Int? = null
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
        ThemeBridge.register(this)
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

    override fun setHtml(html: String, initialScrollOffset: Int) {
        push(html, initialScrollOffset)
    }

    override fun setHtml(html: String, initialScrollOffset: Int, document: VirtualFile?) {
        currentFile = document ?: file
        currentPath = runCatching { (document ?: file)?.toNioPath() }.getOrNull()
        push(html, initialScrollOffset)
    }

    override fun setHtml(html: String, initialScrollOffset: Int, documentPath: Path?) {
        currentPath = documentPath ?: currentPath
        currentFile = documentPath?.let { LocalFileSystem.getInstance().findFileByNioFile(it) } ?: file
        push(html, initialScrollOffset)
    }

    private fun push(html: String, initialScrollOffset: Int = 0) {
        Spike.sourceSent(this)
        val sendNow: Boolean
        val scrollLine: Int?
        synchronized(lock) {
            currentSource = html
            sendNow = ready
            if (initialScrollOffset > 0) {
                pendingScrollLine = offsetToLine(initialScrollOffset)
            }
            scrollLine = if (sendNow) pendingScrollLine else null
            if (sendNow) {
                pendingScrollLine = null
            }
        }
        if (sendNow) {
            setSource(html)
            if (scrollLine != null) {
                scrollToLine(scrollLine)
            }
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
    override fun scrollToMarkdownSrcOffset(offset: Int, smooth: Boolean) {
        val line = offsetToLine(offset) ?: return
        scrollToLine(line)
    }

    override suspend fun scrollTo(editor: Editor, line: Int) {
        scrollToLine(line + 1)
    }

    fun scrollToLine(line: Int) {
        val sendNow: Boolean
        synchronized(lock) {
            if (!ready) {
                pendingScrollLine = line
                sendNow = false
            } else {
                sendNow = true
            }
        }
        if (sendNow) {
            executeJs("if (globalThis.dyamark?.scrollToLine) dyamark.scrollToLine($line);")
        }
    }

    fun offsetToLine(offset: Int): Int? {
        val doc = currentFile?.let { FileDocumentManager.getInstance().getDocument(it) }
        if (doc != null) {
            if (doc.textLength == 0) return 1
            val safeOffset = offset.coerceIn(0, doc.textLength)
            return doc.getLineNumber(safeOffset) + 1
        }
        val text = synchronized(lock) { currentSource } ?: return null
        return computeOffsetToLine(text, offset)
    }

    fun lineToOffset(line: Int): Int? {
        val doc = currentFile?.let { FileDocumentManager.getInstance().getDocument(it) }
        if (doc != null) {
            if (doc.lineCount == 0) return 0
            val lineIndex = (line - 1).coerceIn(0, doc.lineCount - 1)
            return doc.getLineStartOffset(lineIndex)
        }
        val text = synchronized(lock) { currentSource } ?: return null
        return computeLineToOffset(text, line)
    }

    override fun addScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {
        scrollListeners.add(listener)
    }

    override fun removeScrollListener(listener: MarkdownHtmlPanel.ScrollListener) {
        scrollListeners.remove(listener)
    }

    override fun dispose() {
        LOG.info("dyamark ${file?.name}: disposed")
        isPanelDisposed = true
        ThemeBridge.unregister(this)
        if (!query.isDisposed) {
            Disposer.dispose(query)
        }
        if (!browser.isDisposed) {
            Disposer.dispose(browser)
        }
    }

    fun setTheme(json: String) {
        executeJs("if (globalThis.dyamark?.setTheme) dyamark.setTheme(${jsString(json)});")
    }

    private fun setSource(text: String) {
        executeJs("dyamark.setSource(${jsString(text)})")
    }

    private fun handleLink(url: String) {
        if (isWebLink(url)) {
            BrowserUtil.browse(url)
            return
        }
        ApplicationManager.getApplication().invokeLater {
            if (isPanelDisposed) return@invokeLater
            val p = project
            val f = currentFile ?: file
            if (p != null && !p.isDisposed) {
                try {
                    MarkdownLinkOpener.getInstance().openLink(p, url, f)
                    return@invokeLater
                } catch (t: Throwable) {
                    LOG.debug("MarkdownLinkOpener failed to open link $url", t)
                }
            }
            fallbackHandleLink(url)
        }
    }

    private fun fallbackHandleLink(url: String) {
        if (url.startsWith("#")) {
            return
        }
        val cleanUrl = url.removePrefix("file://").substringBefore('#').substringBefore('?')
        val targetFile = resolveRelativeFile(cleanUrl)
        if (targetFile != null) {
            val p = project
            if (p != null && !p.isDisposed) {
                OpenFileDescriptor(p, targetFile).navigate(true)
            } else {
                BrowserUtil.browse(targetFile.path)
            }
        } else {
            LOG.warn("dyamark ${file?.name}: could not resolve link $url")
        }
    }

    internal fun resolveRelativeFile(path: String): VirtualFile? {
        val baseDir = currentFile?.parent
            ?: currentPath?.parent?.let { LocalFileSystem.getInstance().findFileByNioFile(it) }
        return baseDir?.findFileByRelativePath(path)
            ?: LocalFileSystem.getInstance().findFileByPath(path)
            ?: project?.let { it.guessProjectDir()?.findFileByRelativePath(path) }
    }

    private fun onMessage(message: String) {
        if (isPanelDisposed || browser.isDisposed) return
        LOG.info("dyamark ${file?.name}: $message")
        when {
            message.startsWith("ready") -> {
                val (text, scrollLine) = synchronized(lock) {
                    ready = true
                    val line = pendingScrollLine
                    pendingScrollLine = null
                    Pair(currentSource, line)
                }
                setTheme(ThemeBridge.currentThemeJson())
                if (text != null) setSource(text)
                if (scrollLine != null) scrollToLine(scrollLine)
            }
            message.startsWith("scroll ") -> {
                val line = message.removePrefix("scroll ").trim().toIntOrNull()
                if (line != null && line > 0) {
                    val offset = lineToOffset(line) ?: 0
                    ApplicationManager.getApplication().invokeLater {
                        if (isPanelDisposed) return@invokeLater
                        for (listener in scrollListeners) {
                            listener.onScroll(offset)
                        }
                    }
                }
            }
            message.startsWith("edit ") -> {
                val line = message.removePrefix("edit ").trim().toIntOrNull()
                if (line != null && line > 0) {
                    ApplicationManager.getApplication().invokeLater {
                        if (isPanelDisposed) return@invokeLater
                        val p = project ?: return@invokeLater
                        val f = currentFile ?: file ?: return@invokeLater
                        if (p.isDisposed || !f.isValid) return@invokeLater
                        val lineIndex = (line - 1).coerceAtLeast(0)
                        OpenFileDescriptor(p, f, lineIndex, 0).navigate(true)
                    }
                }
            }
            message.startsWith("link ") -> {
                val url = message.removePrefix("link ").trim()
                if (url.isNotEmpty()) {
                    handleLink(url)
                }
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

internal fun isWebLink(url: String): Boolean {
    val lower = url.lowercase()
    return lower.startsWith("http://") ||
           lower.startsWith("https://") ||
           lower.startsWith("mailto:")
}

internal fun computeOffsetToLine(text: String, offset: Int): Int {
    if (text.isEmpty()) return 1
    val safeOffset = offset.coerceIn(0, text.length)
    var line = 1
    for (i in 0 until safeOffset) {
        if (text[i] == '\n') line++
    }
    return line
}

internal fun computeLineToOffset(text: String, line: Int): Int {
    if (text.isEmpty() || line <= 1) return 0
    var curLine = 1
    for (i in text.indices) {
        if (text[i] == '\n') {
            curLine++
            if (curLine == line) {
                return (i + 1).coerceAtMost(text.length)
            }
        }
    }
    return text.length
}
