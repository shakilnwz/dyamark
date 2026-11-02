package dyamark.jetbrains

import com.intellij.ide.util.PropertiesComponent
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.FileEditorManagerListener
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity
import com.intellij.openapi.vfs.VirtualFile
import org.intellij.plugins.markdown.settings.MarkdownSettings
import org.intellij.plugins.markdown.ui.preview.MarkdownHtmlPanelProvider

private val LOG = logger<DyamarkNotification>()

/**
 * Manages the one-time notification prompting the user to try dyamark as the
 * Markdown preview provider when a Markdown file is first opened.
 */
object DyamarkNotification {
    const val NOTIFICATION_GROUP_ID = "Dyamark"
    const val SHOWN_PROPERTY = "dyamark.preview.notification.shown"

    fun shouldNotify(
        isShown: Boolean,
        currentProvider: MarkdownHtmlPanelProvider.ProviderInfo?,
    ): Boolean {
        if (isShown) return false
        if (currentProvider == DyamarkPanelProvider.INFO) return false
        return true
    }

    fun shouldNotify(project: Project): Boolean {
        if (project.isDisposed) return false
        val shown = PropertiesComponent.getInstance().getBoolean(SHOWN_PROPERTY, false)
        val current = MarkdownSettings.getInstance(project).previewPanelProviderInfo
        return shouldNotify(shown, current)
    }

    fun notifyOnce(project: Project) {
        if (!shouldNotify(project)) return

        // Mark as shown so we only ever prompt once across IDE sessions.
        PropertiesComponent.getInstance().setValue(SHOWN_PROPERTY, true)

        val group = NotificationGroupManager.getInstance().getNotificationGroup(NOTIFICATION_GROUP_ID)
        if (group == null) {
            LOG.warn("Notification group '$NOTIFICATION_GROUP_ID' not found")
            return
        }

        val notification = group.createNotification(
            "dyamark Markdown Preview",
            "Use dyamark as the preview for Markdown files and Mermaid sequence diagrams?",
            NotificationType.INFORMATION,
        )

        notification.addAction(NotificationAction.createSimpleExpiring("Use Dyamark") {
            if (!project.isDisposed) {
                val settings = MarkdownSettings.getInstance(project)
                settings.update {
                    it.previewPanelProviderInfo = DyamarkPanelProvider.INFO
                }
            }
        })

        notification.addAction(NotificationAction.createSimpleExpiring("Don't Ask Again") {
            // Already marked shown in PropertiesComponent
        })

        notification.notify(project)
    }
}

fun isMarkdownFile(name: String): Boolean {
    val lower = name.lowercase()
    return lower.endsWith(".md") ||
           lower.endsWith(".markdown") ||
           lower.endsWith(".mdown") ||
           lower.endsWith(".mkdn")
}

fun isMarkdownFile(file: VirtualFile): Boolean = isMarkdownFile(file.name)

/**
 * Listens for Markdown files opened by the user to trigger the one-time notification.
 */
class DyamarkNotificationListener : FileEditorManagerListener {
    override fun fileOpened(source: FileEditorManager, file: VirtualFile) {
        if (isMarkdownFile(file)) {
            DyamarkNotification.notifyOnce(source.project)
        }
    }
}

/**
 * Checks for already-open Markdown files on project startup.
 */
class DyamarkStartupActivity : ProjectActivity {
    override suspend fun execute(project: Project) {
        if (Spike.enabled || Check.enabled) return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            val manager = FileEditorManager.getInstance(project)
            if (manager.openFiles.any { isMarkdownFile(it) }) {
                DyamarkNotification.notifyOnce(project)
            }
        }
    }
}
