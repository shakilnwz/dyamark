package dyamark.jetbrains

import com.intellij.ide.BrowserUtil
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.LogicalPosition
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.project.Project
import com.intellij.openapi.project.guessProjectDir
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.util.registry.Registry
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
import org.intellij.plugins.markdown.ui.preview.MarkdownEditorWithPreview
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanel
import java.awt.event.MouseAdapter
import java.awt.event.MouseEvent
import java.nio.file.Path
import java.util.concurrent.CopyOnWriteArrayList
import javax.swing.JComponent
import javax.swing.SwingUtilities

private val LOG = logger<DyamarkPanel>()

/**
 * One Markdown preview, drawn by dyamark's WebAssembly build in a JCEF
 * browser. The Markdown plugin hands it the raw Markdown through [setHtml].
 */
class DyamarkPanel(private val project: Project?, private val file: VirtualFile?) : MarkdownHtmlPanel {
    private val browser = JBCefBrowser.createBuilder()
        .setEnableOpenDevToolsMenuItem(true)
        .setWindowlessFramerate(frameRate())
        .build()
    private val query = JBCefJSQuery.create(browser as JBCefBrowserBase)

    private val lock = Any()
    private var ready = false
    private var currentSource: String? = null
    private var currentFile: VirtualFile? = file
    private var currentPath: Path? = runCatching { file?.toNioPath() }.getOrNull()
    private var pendingScrollLine: Int? = null
    // The Markdown plugin hands every setHtml the caret's offset; like its
    // JCEF panel, only the first one places the view.
    private var firstUpdate = true
    private val scrollListeners = CopyOnWriteArrayList<MarkdownHtmlPanel.ScrollListener>()
    private var isPanelDisposed = false

    // Until then (System.nanoTime), the editor scrolls this panel asks for
    // aren't sent back to the preview: it is already there, and putting
    // the block's start at the top would undo the reader's scrolling.
    @Volatile
    private var ignoreEditorScrollUntil = 0L

    init {
        Disposer.register(this, browser)
        Disposer.register(this, query)
        // The out-of-process browser the IDE runs ignores the builder's
        // rate and starts at 30; this one it sends on, once it's made.
        browser.cefBrowser.setWindowlessFrameRate(frameRate())

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

    // For Check.
    internal val uiForCheck: JComponent get() = browser.cefBrowser.uiComponent as JComponent
    internal val disposedForCheck get() = isPanelDisposed
    internal val browserDisposedForCheck get() = browser.isDisposed

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
            if (firstUpdate) {
                firstUpdate = false
                if (initialScrollOffset > 0) {
                    pendingScrollLine = offsetToLine(initialScrollOffset)
                }
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
            // The source and this line are sent again on the reloaded
            // page's ready.
            synchronized(lock) {
                pendingScrollLine = offsetToLine(offset)
            }
            browser.cefBrowser.reload()
        }
    }

    @Deprecated("Deprecated in MarkdownHtmlPanel", ReplaceWith(""))
    @Suppress("OVERRIDE_DEPRECATION", "DEPRECATION")
    override fun scrollToMarkdownSrcOffset(offset: Int, smooth: Boolean) {
        val line = offsetToLine(offset) ?: return
        scrollToLine(line)
    }

    /** The editor scrolled, with "synchronise scrolling" on: [line] is its top line, 0-based. */
    override suspend fun scrollTo(editor: Editor, line: Int) {
        if (System.nanoTime() < ignoreEditorScrollUntil) return
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
            Check.event(this, "out scrollToLine $line")
            executeJs("if (globalThis.dyamark?.scrollToLine) dyamark.scrollToLine($line);")
        }
    }

    /**
     * The reader scrolled the preview so that [line], 1-based, starts the
     * block at its top. The Markdown plugin only syncs from the editor to
     * the preview (nothing in 2026.2 listens to a panel's
     * [MarkdownHtmlPanel.ScrollListener]s), so the panel scrolls its
     * editor itself, when that editor's "synchronise scrolling" is on.
     */
    private fun previewScrolled(line: Int) {
        val split = splitEditor()
        if (split != null && split.isAutoScrollPreview) {
            val editor = split.editor
            val lineIndex = (line - 1).coerceIn(0, maxOf(editor.document.lineCount - 1, 0))
            val y = editor.logicalPositionToXY(LogicalPosition(lineIndex, 0)).y
            ignoreEditorScrollUntil = System.nanoTime() + ECHO_NANOS
            val scrolling = editor.scrollingModel
            scrolling.disableAnimation()
            scrolling.scrollVertically(y)
            scrolling.enableAnimation()
            Check.event(this, "editor scrolled to ${lineIndex + 1}")
        }
        val offset = lineToOffset(line) ?: 0
        for (listener in scrollListeners) {
            listener.onScroll(offset)
        }
    }

    /** The split editor showing this preview, if it is in one. */
    private fun splitEditor(): MarkdownEditorWithPreview? {
        val p = project ?: return null
        val f = currentFile ?: file ?: return null
        if (p.isDisposed) return null
        return FileEditorManager.getInstance(p).getAllEditors(f)
            .filterIsInstance<MarkdownEditorWithPreview>()
            .firstOrNull { SwingUtilities.isDescendingFrom(browser.component, it.component) }
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
        Check.event(this, "out setTheme $json")
        executeJs("if (globalThis.dyamark?.setTheme) dyamark.setTheme(${jsString(json)});")
    }

    private fun setSource(text: String) {
        Check.event(this, "out setSource")
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
                    ApplicationManager.getApplication().invokeLater {
                        if (!isPanelDisposed) previewScrolled(line)
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
                        // With the file open in this split editor, navigate
                        // moves the caret but leaves the focus in the preview.
                        val editor = splitEditor()?.editor ?: return@invokeLater
                        browser.cefBrowser.setFocus(false)
                        IdeFocusManager.getInstance(p).requestFocus(editor.contentComponent, true)
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
        Check.event(this, "in $message")
    }

    companion object {
        const val PANEL_CLIENT_PROPERTY = "dyamark.panel"

        // How long after the panel scrolls its editor that editor's
        // scrolls aren't sent back.
        private const val ECHO_NANOS = 500_000_000L
    }
}

/**
 * The off-screen browser's frame rate. The IDE's registry leaves
 * `ide.browser.jcef.osr.framerate` at 0, which is Chromium's default of
 * 30 fps: too few for smooth scrolling, panning and zooming. A rate set
 * there is kept.
 */
private fun frameRate(): Int =
    Registry.intValue(FRAME_RATE_KEY, 0).takeIf { it > 0 } ?: FRAME_RATE

private const val FRAME_RATE_KEY = "ide.browser.jcef.osr.framerate"

// The most Chromium draws an off-screen browser at.
private const val FRAME_RATE = 60

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
