package dyamark.jetbrains

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.PathManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.TextEditorWithPreview
import com.intellij.openapi.project.Project
import com.intellij.openapi.project.guessProjectDir
import com.intellij.openapi.startup.ProjectActivity
import com.intellij.util.concurrency.AppExecutorUtil
import org.intellij.plugins.markdown.settings.MarkdownSettings
import com.intellij.openapi.wm.IdeFocusManager
import java.awt.KeyboardFocusManager
import java.awt.Toolkit
import java.awt.event.InputEvent
import java.awt.event.KeyEvent
import java.awt.image.BufferedImage
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import javax.imageio.ImageIO

/**
 * Step 1's spike, run with `./gradlew runIde -Pspike -PspikeProject=DIR`.
 * It opens DIR/spike.md in dyamark's preview, screenshots it, types into
 * the document, screenshots again, opens spike-2.md to spike-5.md, and
 * logs the times to idea.log. It does nothing without -Ddyamark.spike.
 */
object Spike {
    val enabled = System.getProperty("dyamark.spike") == "true"
    private val LOG = logger<Spike>()

    private var created = 0L
    private var editedAt = 0L
    private var stage = 0

    fun panelCreated(panel: DyamarkPanel) {
        if (!enabled || panel.virtualFile?.name != "spike.md") return
        created = System.nanoTime()
        LOG.info("dyamark spike: panel created")
    }

    fun sourceSent(panel: DyamarkPanel) {
        if (!enabled || panel.virtualFile?.name != "spike.md" || stage != 2) return
        LOG.info("dyamark spike: setHtml ${ms(editedAt)} ms after the edit")
    }

    fun message(panel: DyamarkPanel, message: String) {
        if (!enabled || panel.virtualFile?.name != "spike.md") return
        when {
            message.startsWith("ready") && stage == 0 -> {
                stage = 1
                LOG.info("dyamark spike: ready ${ms(created)} ms after the panel was created")
                later(1500) {
                    shot(panel, "1-ready")
                    edit(panel)
                }
            }
            message == "drawn" && stage == 2 -> {
                stage = 3
                LOG.info("dyamark spike: edit drawn ${ms(editedAt)} ms after typing")
                later(500) {
                    shot(panel, "2-edited")
                    pressKeys(panel)
                }
            }
        }
    }

    private fun edit(panel: DyamarkPanel) {
        val file = panel.virtualFile ?: return
        val document = FileDocumentManager.getInstance().getDocument(file) ?: return
        editedAt = System.nanoTime()
        stage = 2
        WriteCommandAction.runWriteCommandAction(panel.project) {
            document.insertString(0, "# Typed by the spike\n\nThis line was put into the document by the plugin.\n\n")
        }
    }

    /**
     * Posts key presses to the preview through the IDE's event queue, so
     * they meet the keymap (and IdeaVim) the way typed keys do. The page
     * logs every keydown it gets.
     */
    private fun pressKeys(panel: DyamarkPanel) {
        val target = panel.cefBrowser.uiComponent
        LOG.info("dyamark spike: pressing keys on ${target.javaClass.simpleName}")
        IdeFocusManager.getGlobalInstance().requestFocus(target, true)
        panel.cefBrowser.setFocus(true)
        val ctrl = InputEvent.CTRL_DOWN_MASK
        val shift = InputEvent.SHIFT_DOWN_MASK
        val keys = listOf(
            "Down" to Triple(KeyEvent.VK_DOWN, 0, KeyEvent.CHAR_UNDEFINED),
            "PageDown" to Triple(KeyEvent.VK_PAGE_DOWN, 0, KeyEvent.CHAR_UNDEFINED),
            "Space" to Triple(KeyEvent.VK_SPACE, 0, ' '),
            "Home" to Triple(KeyEvent.VK_HOME, 0, KeyEvent.CHAR_UNDEFINED),
            "End" to Triple(KeyEvent.VK_END, 0, KeyEvent.CHAR_UNDEFINED),
            "Home" to Triple(KeyEvent.VK_HOME, 0, KeyEvent.CHAR_UNDEFINED),
            "Tab" to Triple(KeyEvent.VK_TAB, 0, '\t'),
            "Shift+Tab" to Triple(KeyEvent.VK_TAB, shift, '\t'),
            "Tab" to Triple(KeyEvent.VK_TAB, 0, '\t'),
            "j" to Triple(KeyEvent.VK_J, 0, 'j'),
            "R" to Triple(KeyEvent.VK_R, shift, 'R'),
            "Ctrl+=" to Triple(KeyEvent.VK_EQUALS, ctrl, KeyEvent.CHAR_UNDEFINED),
            "Ctrl+-" to Triple(KeyEvent.VK_MINUS, ctrl, KeyEvent.CHAR_UNDEFINED),
            "Ctrl+0" to Triple(KeyEvent.VK_0, ctrl, KeyEvent.CHAR_UNDEFINED),
            "Enter" to Triple(KeyEvent.VK_ENTER, 0, '\n'),
        )
        val queue = Toolkit.getDefaultToolkit().systemEventQueue
        keys.forEachIndexed { i, (name, key) ->
            later(400L * (i + 1)) {
                val (code, mods, ch) = key
                val now = System.currentTimeMillis()
                LOG.info("dyamark spike: press $name (focus ${KeyboardFocusManager.getCurrentKeyboardFocusManager().focusOwner?.javaClass?.simpleName})")
                queue.postEvent(KeyEvent(target, KeyEvent.KEY_PRESSED, now, mods, code, ch))
                if (ch != KeyEvent.CHAR_UNDEFINED) {
                    queue.postEvent(KeyEvent(target, KeyEvent.KEY_TYPED, now, mods, KeyEvent.VK_UNDEFINED, ch))
                }
                queue.postEvent(KeyEvent(target, KeyEvent.KEY_RELEASED, now + 30, mods, code, ch))
            }
        }
        later(400L * (keys.size + 3)) {
            shot(panel, "3-keys")
            panel.project?.let(::openMore)
        }
    }

    private fun openMore(project: Project) {
        val dir = project.guessProjectDir() ?: return
        for (i in 2..5) {
            val f = dir.findChild("spike-$i.md") ?: continue
            FileEditorManager.getInstance(project).openFile(f, true)
        }
        later(15000) {
            LOG.info("dyamark spike: done")
            Files.writeString(Path.of(PathManager.getLogPath(), "dyamark-spike-done"), "done\n")
        }
    }

    private fun shot(panel: DyamarkPanel, name: String) {
        val c = panel.component
        if (c.width <= 0 || c.height <= 0) {
            LOG.info("dyamark spike: no screenshot $name, the panel is ${c.width}x${c.height}")
            return
        }
        val image = BufferedImage(c.width, c.height, BufferedImage.TYPE_INT_RGB)
        val g = image.createGraphics()
        c.paint(g)
        g.dispose()
        val out = Path.of(PathManager.getLogPath(), "dyamark-$name.png")
        ImageIO.write(image, "png", out.toFile())
        LOG.info("dyamark spike: screenshot $out (${c.width}x${c.height})")
    }

    private fun later(ms: Long, f: () -> Unit) {
        AppExecutorUtil.getAppScheduledExecutorService().schedule(
            { ApplicationManager.getApplication().invokeLater(f) }, ms, TimeUnit.MILLISECONDS,
        )
    }

    private fun ms(since: Long) = (System.nanoTime() - since) / 1_000_000
}

/** Picks dyamark's preview and opens spike.md, in spike runs only. */
class SpikeStartup : ProjectActivity {
    override suspend fun execute(project: Project) {
        if (!Spike.enabled) return
        ApplicationManager.getApplication().invokeLater {
            val settings = MarkdownSettings.getInstance(project)
            settings.previewPanelProviderInfo = DyamarkPanelProvider.INFO
            settings.splitLayout = TextEditorWithPreview.Layout.SHOW_EDITOR_AND_PREVIEW
            val f = project.guessProjectDir()?.findChild("spike.md") ?: return@invokeLater
            FileEditorManager.getInstance(project).openFile(f, true)
        }
    }
}
