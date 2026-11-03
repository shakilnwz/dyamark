package dyamark.jetbrains

import com.intellij.ide.util.PropertiesComponent
import com.intellij.notification.Notification
import com.intellij.notification.Notifications
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.ModalityState
import com.intellij.openapi.application.PathManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.LogicalPosition
import com.intellij.openapi.editor.colors.EditorColorsManager
import com.intellij.openapi.editor.colors.EditorColorsScheme
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.TextEditorWithPreview
import com.intellij.openapi.project.DumbService
import com.intellij.openapi.project.Project
import com.intellij.openapi.project.guessProjectDir
import com.intellij.openapi.startup.ProjectActivity
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.wm.WindowManager
import com.intellij.ui.ColorUtil
import org.intellij.plugins.markdown.settings.MarkdownSettings
import org.intellij.plugins.markdown.ui.preview.MarkdownEditorWithPreview
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanelProvider
import java.awt.Color
import java.awt.Component
import java.awt.Point
import java.awt.Rectangle
import java.awt.Toolkit
import java.awt.event.InputEvent
import java.awt.event.KeyEvent
import java.awt.event.MouseEvent
import java.awt.event.MouseWheelEvent
import java.awt.image.BufferedImage
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.CopyOnWriteArrayList
import javax.imageio.ImageIO
import javax.swing.JComponent
import kotlin.concurrent.thread
import kotlin.math.abs

/**
 * Checks steps 4 to 7 of docs/plans/2026-11-01-phpstorm-plugin.md in a
 * running IDE: `./gradlew runIde -Pcheck`. It opens the files of
 * jetbrains/check/ (copied fresh to build/check-project/), works the IDE
 * and the preview as a reader would, and writes each result to idea.log
 * and to dyamark-check.txt beside it, with screenshots. Then it quits the
 * IDE. It does nothing without -Ddyamark.check.
 */
object Check {
    val enabled = System.getProperty("dyamark.check") == "true"
    private val LOG = logger<Check>()

    /** What a panel was told, "in MESSAGE", or did, "out CALL …". */
    private class Event(val panel: DyamarkPanel, val text: String)

    private val events = ArrayList<Event>()
    private val results = CopyOnWriteArrayList<String>()

    fun event(panel: DyamarkPanel, text: String) {
        if (!enabled) return
        synchronized(events) { events.add(Event(panel, text)) }
    }

    fun start(project: Project) {
        thread(name = "dyamark check", isDaemon = true) {
            try {
                run(project)
            } catch (t: Throwable) {
                result(false, 0, "the check stopped: $t")
                LOG.warn("dyamark check stopped", t)
            }
            finish()
        }
    }

    private fun run(project: Project) {
        DumbService.getInstance(project).waitForSmartMode()
        val dir = edt { project.guessProjectDir() } ?: error("no project directory")
        val checkMd = dir.findChild("check.md") ?: error("no check.md")
        val apiMd = dir.findChild("api.md") ?: error("no api.md")
        val notifyMd = dir.findChild("notify.md") ?: error("no notify.md")
        val files = FileEditorManager.getInstance(project)
        val settings = MarkdownSettings.getInstance(project)
        edt {
            PropertiesComponent.getInstance().setValue(DyamarkNotification.SHOWN_PROPERTY, true)
            settings.previewPanelProviderInfo = DyamarkPanelProvider.INFO
            settings.splitLayout = TextEditorWithPreview.Layout.SHOW_EDITOR_AND_PREVIEW
            settings.isAutoScrollEnabled = true
            files.openFiles.forEach(files::closeFile)
        }
        val text = edt { String(checkMd.contentsToByteArray()) }

        // Step 4: the preview shows, follows typing, and goes with its tab.
        var m = mark()
        edt { files.openFile(checkMd, true) }
        val firstDrawn = waitFor(m, 30_000) { it.panel.virtualFile == checkMd && it.text == "in drawn" }
        result(firstDrawn != null, 4, "opening check.md shows dyamark's preview")
        var panel = firstDrawn?.panel ?: return
        pause(1500)

        val editor = edt { split(files, checkMd)?.editor } ?: error("check.md isn't in a split editor")
        m = mark()
        edt {
            WriteCommandAction.runWriteCommandAction(project) {
                editor.document.insertString(editor.document.textLength, "\nTyped by the check.\n")
            }
        }
        result(waitFor(m, 5000) { it.panel === panel && it.text == "in drawn" } != null, 4, "typing is drawn")

        m = mark()
        val closed = panel
        edt { files.closeFile(checkMd) }
        val gone = waitUntil(5000) { closed.disposedForCheck && closed.browserDisposedForCheck && closed !in ThemeBridge.openPanels() }
        result(gone, 4, "closing the tab disposes the panel and its browser")
        edt { files.openFile(checkMd, true) }
        val reopened = waitFor(m, 20_000) { it.panel.virtualFile == checkMd && it.text == "in drawn" }
        result(reopened != null, 4, "reopening it shows the preview again")
        panel = reopened?.panel ?: return
        result(ThemeBridge.openPanels() == listOf(panel), 4, "one live panel, the shown one", "${ThemeBridge.openPanels().size} live")
        pause(1500)
        val ed = edt { split(files, checkMd)?.editor } ?: error("check.md isn't in a split editor")

        // Step 5: colours from the editor scheme, live.
        val colors = EditorColorsManager.getInstance()
        val original = edt { colors.globalScheme }
        restoreScheme = original
        val schemes = edt { colors.allSchemes.toList() }
        val light = schemes.firstOrNull { it.name.endsWith("Light") } ?: schemes.first { !ColorUtil.isDark(it.defaultBackground) }
        val dark = schemes.firstOrNull { it.name.endsWith("Dark") && !it.name.contains("Islands") }
            ?: schemes.first { ColorUtil.isDark(it.defaultBackground) }
        note("schemes: ${light.name} and ${dark.name}; the IDE had ${original.name}")
        val initLine = lineOf(text, "%%{init")
        // Light, then dark: step 6 runs in the dark scheme.
        for ((name, scheme) in listOf("light" to light, "dark" to dark)) {
            m = mark()
            edt { colors.setGlobalScheme(scheme) }
            val bg = hex(scheme.defaultBackground)
            val sent = waitFor(m, 5000) { it.panel.virtualFile == checkMd && it.text.startsWith("out setTheme") && it.text.contains(bg) }
            result(sent != null, 5, "switching to ${scheme.name} sends its colours to the preview", bg)
            // The Markdown plugin may build a new preview for the new scheme.
            waitFor(m, 3000) { it.panel.virtualFile == checkMd && it.text == "in drawn" }
            val now = live(checkMd) ?: return
            note("$name: the Markdown plugin ${if (now === panel) "kept" else "rebuilt"} the preview")
            panel = now
            edt { ed.scrollingModel.scrollVertically(0) }
            pause(1200)
            val page = shot(panel.uiForCheck, "check-$name")
            val corner = Color(page.image.getRGB(3, 3))
            result(near(corner, scheme.defaultBackground, 6), 5, "$name: the preview's page is the scheme's background", "${hex(corner)}, want $bg")
            val accent = Color.decode(accentOf(scheme))
            val linkPx = count(page.image, accent, 40)
            result(linkPx > 50, 5, "$name: its links are in the scheme's link colour", "${hex(accent)}, $linkPx px")
            edt { split(files, checkMd)?.component }?.let { shot(it, "check-split-$name") }

            panel.scrollToLine(initLine)
            pause(1200)
            val red = count(shot(panel.uiForCheck, "check-$name-init").image, Color(0xd0, 0x30, 0x30), 30)
            result(red > 300, 5, "$name: the diagram with its own %%{init}%% actorBkg keeps it", "$red px of #d03030")
            panel.scrollToLine(1)
        }

        // Step 6: typing doesn't move the preview to the caret.
        panel = live(checkMd) ?: return
        edt { ed.scrollingModel.scrollVertically(0) }
        pause(1000)
        m = mark()
        edt {
            ed.caretModel.moveToLogicalPosition(LogicalPosition(120, 0))
            WriteCommandAction.runWriteCommandAction(project) {
                ed.document.insertString(ed.caretModel.offset, "Typed ")
            }
        }
        val typedDrawn = waitFor(m, 5000) { it.panel === panel && it.text == "in drawn" }
        pause(600)
        val jumps = since(m).filter { it.panel === panel && it.text.startsWith("out scrollToLine") }
        result(typedDrawn != null && jumps.isEmpty(), 6, "typing on line 121, with the editor at the top, leaves the preview where it is",
            jumps.joinToString { it.text }.ifEmpty { "nothing sent" })

        // Editor to preview.
        m = mark()
        edt { ed.scrollingModel.scrollVertically(ed.logicalPositionToXY(LogicalPosition(79, 0)).y) }
        pause(1200)
        val sentLine = since(m).lastOrNull { it.panel === panel && it.text.startsWith("out scrollToLine") }?.text?.substringAfterLast(' ')?.toDoubleOrNull()?.toInt()
        result(sentLine != null && abs(sentLine - 80) <= 1, 6, "scrolling the editor to line 80 scrolls the preview there", "scrollToLine $sentLine")
        val toldBack = since(m).filter { it.panel === panel && it.text.startsWith("in scroll ") }
        result(toldBack.isEmpty(), 6, "and the preview doesn't tell it back", toldBack.joinToString { it.text })

        // The frame rate the page gets, and what a wheel notch comes to in
        // it: the eased scrolling and the zooms are tuned for 120 px.
        m = mark()
        panel.cefBrowser.executeJavaScript(
            "{ let n = 0; const end = performance.now() + 1000;" +
                " const tick = (t) => { if (t < end) { n++; requestAnimationFrame(tick); } else dyamarkSend('log fps ' + n); };" +
                " requestAnimationFrame(tick); }",
            WebResources.INDEX, 0,
        )
        val fps = waitFor(m, 5000) { it.panel === panel && it.text.startsWith("in log fps ") }?.text?.substringAfterLast(' ')?.toIntOrNull()
        result(fps != null && fps >= 50, 6, "the preview draws at 60 fps, not Chromium's off-screen 30", "$fps frames in a second")
        panel.cefBrowser.executeJavaScript(
            "addEventListener('wheel', (e) => dyamarkSend('log wheel ' + e.deltaY + ' mode ' + e.deltaMode + ' dpr ' + devicePixelRatio)," +
                " { capture: true, passive: true });",
            WebResources.INDEX, 0,
        )
        pause(300)

        // Preview to editor.
        edt { ed.scrollingModel.scrollVertically(0) }
        pause(1200)
        m = mark()
        wheel(panel, notches = 6)
        waitFor(m, 2000) { it.panel === panel && it.text.startsWith("in log wheel ") }?.let { note("a wheel notch in the page: ${it.text.removePrefix("in log ")}") }
        pause(1500)
        val eased = since(m).filter { it.panel === panel && it.text.startsWith("in scroll ") }
        note("6 notches told ${eased.size} top lines: ${eased.joinToString(" ") { it.text.substringAfterLast(' ') }}")
        val scrolled = since(m).lastOrNull { it.panel === panel && it.text.startsWith("in scroll ") }?.text?.substringAfterLast(' ')?.toDoubleOrNull()?.toInt()
        val top = edt { topLine(ed) }
        result(scrolled != null, 6, "the mouse wheel on the preview scrolls it, and it tells the IDE", "scroll $scrolled")
        result(scrolled != null && abs(top - scrolled) <= 1, 6, "and the editor follows: its top line is the preview's", "editor $top, preview $scrolled")
        val echo = since(m).filter { it.panel === panel && it.text.startsWith("out scrollToLine") }
        result(echo.isEmpty(), 6, "and the editor's scroll isn't sent back to the preview", echo.joinToString { it.text })

        // Links, found by their colour at the top of the page.
        edt { ed.scrollingModel.scrollVertically(0) }
        pause(1500)
        val page = shot(panel.uiForCheck, "check-links")
        val links = accentRows(page.image, Color.decode(accentOf(edt { colors.globalScheme })))
        note("link rows at the top: ${links.joinToString { "${it.x},${it.y} ${it.width}x${it.height}" }} (image px, scale ${page.scale})")
        if (links.size < 2) {
            result(false, 6, "found the two links at the top of check.md", "${links.size} found")
        } else {
            val notes = lineOf(text, "## Notes and follow-ups")
            m = mark()
            click(panel, links[0], page.scale)
            pause(1500)
            val anchor = since(m).lastOrNull { it.panel === panel && it.text.startsWith("in scroll ") }?.text?.substringAfterLast(' ')?.toDoubleOrNull()?.toInt()
            result(anchor == notes, 6, "a #anchor link scrolls the preview to its heading", "scroll $anchor, heading on $notes")
            result(since(m).none { it.text.startsWith("in link") }, 6, "and isn't sent to the IDE as a link")
            val shown = edt { visibleLines(ed) }
            result(notes in shown, 6, "and the editor follows it", "editor shows $shown")

            edt { ed.scrollingModel.scrollVertically(0) }
            pause(1500)
            m = mark()
            click(panel, links[1], page.scale)
            val link = waitFor(m, 3000) { it.panel === panel && it.text.startsWith("in link ") }
            result(link?.text == "in link api.md", 6, "a relative link is sent to the IDE", link?.text ?: "nothing")
            val opened = waitUntil(5000) { edt { files.selectedFiles.firstOrNull() } == apiMd }
            result(opened, 6, "and the IDE opens api.md")
            note("not run: a web link, which would open the desktop's browser")
            m = mark()
            edt { files.openFile(checkMd, true) }
            pause(1500)
        }

        // E in the diagram viewer.
        panel = live(checkMd) ?: return
        val fence = lineOf(text, "```mermaid")
        panel.scrollToLine(fence + 1)
        pause(1200)
        m = mark()
        val ui = panel.uiForCheck
        click(panel, Rectangle(ui.width / 2 - 1, 70, 2, 2), 1.0)
        pause(1000)
        // Keys and focus need the IDE's window to be the desktop's active
        // one, which the person at the desktop can take away.
        val active = waitUntil(10_000) { edt { activate(project) } }
        edt { panel.requestFocus() }
        pause(300)
        note("before E: ${edt { focusState() }}")
        if (active) {
            press(panel, KeyEvent.VK_E, 'e')
        } else {
            note("the IDE's window isn't active: E goes straight to the browser")
            pressInBrowser(panel, KeyEvent.VK_E, 'e')
        }
        val edit = waitFor(m, 3000) { it.panel === panel && it.text.startsWith("in edit ") }?.text?.substringAfterLast(' ')?.toIntOrNull()
        pause(1000)
        result(edit == fence + 1, 6, "E in the diagram viewer asks to edit the diagram's first line", "edit $edit, fence on $fence")
        val caret = edt { ed.caretModel.logicalPosition.line + 1 }
        result(edit != null && caret == edit, 6, "and the caret is put on it", "caret on $caret")
        val focused = edt { ed.contentComponent.isFocusOwner }
        note("after E: ${edt { focusState() }}")
        if (active && edt { activate(project) }) {
            result(focused, 6, "and the editor has the focus")
        } else {
            note("not checked: whether the editor has the focus, as the IDE's window wasn't active")
        }

        // Step 7: the Preview choice and the one-time notification.
        val jcef = MarkdownHtmlPanelProvider.getProviders().map { it.providerInfo }
            .firstOrNull { it.className.contains("JCEF", ignoreCase = true) }
        note("providers: ${MarkdownHtmlPanelProvider.getProviders().joinToString { it.providerInfo.name }}")
        if (jcef == null) {
            result(false, 7, "the IDE offers its JCEF preview")
            return
        }
        edt { files.openFile(apiMd, false) }
        pause(800)
        edt { settings.update { it.previewPanelProviderInfo = jcef } }
        val swapped = waitUntil(8000) { ThemeBridge.openPanels().isEmpty() }
        result(swapped, 7, "choosing ${jcef.name} swaps dyamark's preview out", "${ThemeBridge.openPanels().size} dyamark panels left")

        val shownNotes = CopyOnWriteArrayList<Notification>()
        val connection = project.messageBus.connect()
        connection.subscribe(Notifications.TOPIC, object : Notifications {
            override fun notify(notification: Notification) {
                if (notification.groupId == DyamarkNotification.NOTIFICATION_GROUP_ID) shownNotes.add(notification)
            }
        })
        try {
            edt { PropertiesComponent.getInstance().setValue(DyamarkNotification.SHOWN_PROPERTY, false) }
            edt { files.openFile(notifyMd, true) }
            result(waitUntil(5000) { shownNotes.isNotEmpty() }, 7, "opening a Markdown file shows the notification")
            edt { files.closeFile(apiMd) }
            edt { files.openFile(apiMd, true) }
            pause(2000)
            result(shownNotes.size == 1, 7, "once: opening the next one doesn't", "${shownNotes.size} shown")
            val notification = shownNotes.firstOrNull() ?: return
            val use = notification.actions.firstOrNull { it.templatePresentation.text == "Use Dyamark" }
            if (use == null) {
                result(false, 7, "the notification offers Use Dyamark")
                return
            }
            m = mark()
            edt { Notification.fire(notification, use, null) }
            result(edt { settings.previewPanelProviderInfo } == DyamarkPanelProvider.INFO, 7, "its Use Dyamark makes dyamark the preview")
            val back = waitFor(m, 20_000) { it.panel.virtualFile == apiMd && it.text == "in drawn" }
            result(back != null, 7, "and swaps the open preview (api.md) back to dyamark")
            edt { files.openFile(checkMd, true) }
            val next = waitFor(m, 20_000) { it.panel.virtualFile == checkMd && it.text == "in drawn" }
            result(next != null, 7, "and the next one shown (check.md) is dyamark's too")
        } finally {
            Disposer.dispose(connection)
        }
    }

    private var restoreScheme: EditorColorsScheme? = null

    /** The newest live preview of [file]. */
    private fun live(file: VirtualFile): DyamarkPanel? =
        ThemeBridge.openPanels().lastOrNull { it.virtualFile == file && !it.disposedForCheck }

    private fun finish() {
        restoreScheme?.let { scheme -> runCatching { edt { EditorColorsManager.getInstance().setGlobalScheme(scheme) } } }
        val failed = results.count { it.startsWith("FAIL") }
        val summary = if (failed == 0) "all ok" else "$failed failed"
        Files.writeString(Path.of(PathManager.getLogPath(), "dyamark-check.txt"), results.joinToString("\n", postfix = "\n$summary\n"))
        LOG.info("dyamark check: done, $summary")
        ApplicationManager.getApplication().invokeLater { ApplicationManager.getApplication().exit(true, true, false) }
    }

    private fun result(passed: Boolean, step: Int, what: String, detail: String = "") {
        val line = (if (passed) "ok   " else "FAIL ") + "step $step: $what" + (if (detail.isNotEmpty()) " ($detail)" else "")
        results.add(line)
        LOG.info("dyamark check: $line")
    }

    private fun note(text: String) {
        results.add("     $text")
        LOG.info("dyamark check: $text")
    }

    private fun mark() = synchronized(events) { events.size }

    private fun since(mark: Int): List<Event> = synchronized(events) { events.drop(mark) }

    private fun waitFor(mark: Int, ms: Long, test: (Event) -> Boolean): Event? {
        val end = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < end) {
            since(mark).firstOrNull(test)?.let { return it }
            pause(25)
        }
        return null
    }

    private fun waitUntil(ms: Long, test: () -> Boolean): Boolean {
        val end = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < end) {
            if (test()) return true
            pause(50)
        }
        return false
    }

    private fun pause(ms: Long) = Thread.sleep(ms)

    private fun <T> edt(f: () -> T): T {
        var result: Result<T>? = null
        ApplicationManager.getApplication().invokeAndWait({ result = runCatching(f) }, ModalityState.any())
        return result!!.getOrThrow()
    }

    private fun split(files: FileEditorManager, file: VirtualFile): MarkdownEditorWithPreview? =
        files.getAllEditors(file).filterIsInstance<MarkdownEditorWithPreview>().firstOrNull()

    /** Asks for the project's window to be brought forward; reports whether it is active. */
    private fun activate(project: Project): Boolean {
        val frame = WindowManager.getInstance().getFrame(project) ?: return false
        if (!frame.isActive) {
            frame.toFront()
            frame.requestFocus()
        }
        return frame.isActive
    }

    /** Sends a key to the browser itself, past Swing's focus. */
    private fun pressInBrowser(panel: DyamarkPanel, code: Int, ch: Char) {
        val target = panel.uiForCheck
        val now = System.currentTimeMillis()
        panel.cefBrowser.setFocus(true)
        panel.cefBrowser.sendKeyEvent(KeyEvent(target, KeyEvent.KEY_PRESSED, now, 0, code, ch))
        panel.cefBrowser.sendKeyEvent(KeyEvent(target, KeyEvent.KEY_TYPED, now, 0, KeyEvent.VK_UNDEFINED, ch))
        panel.cefBrowser.sendKeyEvent(KeyEvent(target, KeyEvent.KEY_RELEASED, now + 30, 0, code, ch))
    }

    private fun focusState(): String {
        val focus = java.awt.KeyboardFocusManager.getCurrentKeyboardFocusManager()
        fun name(c: Component?) = c?.javaClass?.simpleName ?: "none"
        return "focus owner ${name(focus.focusOwner)}, permanent ${name(focus.permanentFocusOwner)}, " +
            "active window ${name(focus.activeWindow)}"
    }

    /** The editor's first visible line, 1-based. */
    private fun topLine(editor: Editor): Int =
        editor.xyToLogicalPosition(Point(0, editor.scrollingModel.verticalScrollOffset)).line + 1

    private fun visibleLines(editor: Editor): IntRange {
        val area = editor.scrollingModel.visibleArea
        val first = editor.xyToLogicalPosition(Point(0, area.y)).line + 1
        val last = editor.xyToLogicalPosition(Point(0, area.y + area.height - 1)).line + 1
        return first..last
    }

    private fun lineOf(text: String, prefix: String) = text.lines().indexOfFirst { it.startsWith(prefix) } + 1

    private fun accentOf(scheme: EditorColorsScheme): String =
        Regex("\"accent\":\"(#[0-9a-f]{6})\"").find(ThemeBridge.themeJson(scheme))!!.groupValues[1]

    private fun hex(c: Color) = String.format("#%02x%02x%02x", c.red, c.green, c.blue)

    private fun near(a: Color, b: Color, tolerance: Int) =
        abs(a.red - b.red) <= tolerance && abs(a.green - b.green) <= tolerance && abs(a.blue - b.blue) <= tolerance

    private fun count(image: BufferedImage, color: Color, tolerance: Int): Int {
        var n = 0
        for (y in 0 until image.height) for (x in 0 until image.width) {
            if (near(Color(image.getRGB(x, y)), color, tolerance)) n++
        }
        return n
    }

    /** The rows of text in [color] in the top half of [image], top first. */
    private fun accentRows(image: BufferedImage, color: Color): List<Rectangle> {
        val rows = ArrayList<Rectangle>()
        var current: Rectangle? = null
        var lastY = -100
        for (y in 0 until image.height / 2) {
            var minX = -1
            var maxX = -1
            for (x in 0 until image.width) {
                if (near(Color(image.getRGB(x, y)), color, 40)) {
                    if (minX < 0) minX = x
                    maxX = x
                }
            }
            if (minX < 0) continue
            val r = Rectangle(minX, y, maxX - minX + 1, 1)
            if (current != null && y - lastY <= 4) current.add(r) else {
                current = r
                rows.add(r)
            }
            lastY = y
        }
        return rows.filter { it.height >= 5 }
    }

    private class Shot(val image: BufferedImage, val scale: Double)

    /** Paints [c] at the screen's scale, and saves it beside idea.log. */
    private fun shot(c: Component, name: String): Shot = edt {
        val scale = c.graphicsConfiguration?.defaultTransform?.scaleX ?: 1.0
        val image = BufferedImage(maxOf((c.width * scale).toInt(), 1), maxOf((c.height * scale).toInt(), 1), BufferedImage.TYPE_INT_RGB)
        val g = image.createGraphics()
        g.scale(scale, scale)
        c.paint(g)
        g.dispose()
        ImageIO.write(image, "png", Path.of(PathManager.getLogPath(), "dyamark-$name.png").toFile())
        Shot(image, scale)
    }

    /** Clicks the middle of [r], in image pixels at [scale], in the preview. */
    private fun click(panel: DyamarkPanel, r: Rectangle, scale: Double) {
        val target = panel.uiForCheck
        val x = (r.centerX / scale).toInt()
        val y = (r.centerY / scale).toInt()
        val queue = Toolkit.getDefaultToolkit().systemEventQueue
        val now = System.currentTimeMillis()
        queue.postEvent(MouseEvent(target, MouseEvent.MOUSE_MOVED, now, 0, x, y, 0, false, MouseEvent.NOBUTTON))
        pause(100)
        queue.postEvent(MouseEvent(target, MouseEvent.MOUSE_PRESSED, now + 100, InputEvent.BUTTON1_DOWN_MASK, x, y, 1, false, MouseEvent.BUTTON1))
        pause(80)
        queue.postEvent(MouseEvent(target, MouseEvent.MOUSE_RELEASED, now + 180, 0, x, y, 1, false, MouseEvent.BUTTON1))
        queue.postEvent(MouseEvent(target, MouseEvent.MOUSE_CLICKED, now + 180, 0, x, y, 1, false, MouseEvent.BUTTON1))
    }

    private fun wheel(panel: DyamarkPanel, notches: Int) {
        val target = panel.uiForCheck
        val queue = Toolkit.getDefaultToolkit().systemEventQueue
        val x = target.width / 2
        val y = target.height / 2
        repeat(notches) {
            queue.postEvent(MouseWheelEvent(target, MouseEvent.MOUSE_WHEEL, System.currentTimeMillis(), 0, x, y, 0, false,
                MouseWheelEvent.WHEEL_UNIT_SCROLL, 3, 1))
            pause(60)
        }
    }

    private fun press(panel: DyamarkPanel, code: Int, ch: Char) {
        val target = panel.uiForCheck
        val queue = Toolkit.getDefaultToolkit().systemEventQueue
        val now = System.currentTimeMillis()
        queue.postEvent(KeyEvent(target, KeyEvent.KEY_PRESSED, now, 0, code, ch))
        queue.postEvent(KeyEvent(target, KeyEvent.KEY_TYPED, now, 0, KeyEvent.VK_UNDEFINED, ch))
        queue.postEvent(KeyEvent(target, KeyEvent.KEY_RELEASED, now + 30, 0, code, ch))
    }
}

/** Starts the check, in check runs only. */
class CheckStartup : ProjectActivity {
    override suspend fun execute(project: Project) {
        if (Check.enabled) Check.start(project)
    }
}
