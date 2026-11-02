package dyamark.jetbrains

import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.PlatformCoreDataKeys
import java.awt.Component
import javax.swing.JComponent

abstract class DyamarkZoomAction : AnAction() {
    override fun getActionUpdateThread(): ActionUpdateThread = ActionUpdateThread.EDT

    override fun update(e: AnActionEvent) {
        val panel = findFocusedPanel(e)
        e.presentation.isEnabledAndVisible = panel != null
    }

    protected fun findFocusedPanel(e: AnActionEvent): DyamarkPanel? {
        var c: Component? = e.getData(PlatformCoreDataKeys.CONTEXT_COMPONENT)
        while (c != null) {
            if (c is JComponent) {
                val p = c.getClientProperty(DyamarkPanel.PANEL_CLIENT_PROPERTY) as? DyamarkPanel
                if (p != null) return p
            }
            c = c.parent
        }
        return null
    }
}

class ZoomInAction : DyamarkZoomAction() {
    override fun actionPerformed(e: AnActionEvent) {
        findFocusedPanel(e)?.zoomIn()
    }
}

class ZoomOutAction : DyamarkZoomAction() {
    override fun actionPerformed(e: AnActionEvent) {
        findFocusedPanel(e)?.zoomOut()
    }
}

class ResetZoomAction : DyamarkZoomAction() {
    override fun actionPerformed(e: AnActionEvent) {
        findFocusedPanel(e)?.resetZoom()
    }
}
