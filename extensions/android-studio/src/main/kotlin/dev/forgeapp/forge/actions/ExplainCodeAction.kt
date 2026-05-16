package dev.forgeapp.forge.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.wm.ToolWindowManager
import dev.forgeapp.forge.api.ForgeClient
import dev.forgeapp.forge.ui.ForgePanel

/**
 * Right-click → Forge → Explain Selection
 *
 * Enabled only when text is selected in the editor.
 * Sends "Explain this code:" to the daemon and posts the result in the
 * Forge tool window.
 */
class ExplainCodeAction : AnAction() {

    override fun update(e: AnActionEvent) {
        val editor    = e.getData(CommonDataKeys.EDITOR)
        val hasSelection = editor?.selectionModel?.hasSelection() == true
        e.presentation.isEnabled  = hasSelection && e.project != null
        e.presentation.isVisible  = e.project != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project   = e.project ?: return
        val editor    = e.getData(CommonDataKeys.EDITOR) ?: return
        val selection = editor.selectionModel.selectedText ?: return
        val filePath  = editor.virtualFile?.path

        val client = ForgeClient.instance

        object : Task.Backgroundable(project, "Asking Forge to explain…", false) {
            override fun run(indicator: ProgressIndicator) {
                // Connect if needed
                if (!client.isConnected()) {
                    indicator.text = "Connecting to Forge daemon…"
                    val connectResult = client.connect(project.basePath ?: "")
                    if (connectResult.isFailure) {
                        showNotification(
                            project,
                            "Forge daemon not running",
                            "Start it with: forge start",
                            NotificationType.WARNING
                        )
                        return
                    }
                }

                // Ask
                indicator.text = "Waiting for explanation…"
                val result = client.ask(
                    instruction = "Explain this code:",
                    selection   = selection,
                    filePath    = filePath
                )

                result.fold(
                    onSuccess = { response ->
                        ApplicationManager.getApplication().invokeLater {
                            postToToolWindow(project, selection, response)
                        }
                    },
                    onFailure = { ex ->
                        if (isConnectionError(ex)) {
                            client.disconnect()
                            showNotification(
                                project,
                                "Forge daemon not running",
                                "Start it with: forge start",
                                NotificationType.WARNING
                            )
                        } else {
                            showNotification(
                                project,
                                "Forge error",
                                ex.message ?: "Unknown error",
                                NotificationType.ERROR
                            )
                        }
                    }
                )
            }
        }.queue()
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    private fun postToToolWindow(project: com.intellij.openapi.project.Project, selection: String, response: String) {
        val tw = ToolWindowManager.getInstance(project).getToolWindow("Forge")
        tw?.activate(null)

        val forgePanel = tw?.contentManager?.contents
            ?.mapNotNull { it.component as? ForgePanel }
            ?.firstOrNull()

        if (forgePanel != null) {
            val preview = if (selection.length > 80) selection.take(80) + "…" else selection
            forgePanel.appendMessage("You: Explain this code: $preview")
            forgePanel.appendMessage("Forge: $response\n")
        } else {
            com.intellij.openapi.ui.Messages.showInfoMessage(project, response, "Forge — Explanation")
        }
    }

    private fun showNotification(
        project: com.intellij.openapi.project.Project,
        title: String,
        content: String,
        type: NotificationType
    ) {
        ApplicationManager.getApplication().invokeLater {
            NotificationGroupManager.getInstance()
                .getNotificationGroup("Forge Notifications")
                .createNotification(title, content, type)
                .notify(project)
        }
    }

    private fun isConnectionError(ex: Throwable): Boolean =
        ex is java.io.IOException && (
            ex.message?.contains("refused", ignoreCase = true) == true ||
            ex.message?.contains("reach Forge daemon", ignoreCase = true) == true
        )
}
