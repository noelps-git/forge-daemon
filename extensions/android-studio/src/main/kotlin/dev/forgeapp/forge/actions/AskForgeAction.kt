package dev.forgeapp.forge.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.wm.ToolWindowManager
import dev.forgeapp.forge.api.ForgeClient
import dev.forgeapp.forge.ui.ForgeToolWindowFactory

class AskForgeAction : AnAction() {

    override fun update(e: AnActionEvent) {
        // Action is always visible; it handles the "no editor" case at runtime.
        e.presentation.isEnabled = e.project != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val editor  = e.getData(CommonDataKeys.EDITOR)

        val selection = editor?.selectionModel?.selectedText
        val filePath  = editor?.virtualFile?.path

        // Prompt the user for their question (on the EDT – that is fine for showInputDialog)
        val question = Messages.showInputDialog(
            project,
            "What would you like to ask Forge?",
            "Ask Forge AI",
            Messages.getQuestionIcon(),
            "",
            null
        )?.trim()

        if (question.isNullOrBlank()) return

        ensureConnectedAndAsk(project, question, selection, filePath)
    }

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    /**
     * Connects to the daemon if necessary, then sends the question in a
     * background task so the EDT is never blocked.
     */
    internal fun ensureConnectedAndAsk(
        project: Project,
        instruction: String,
        selection: String?,
        filePath: String?
    ) {
        val client = ForgeClient.instance

        object : Task.Backgroundable(project, "Asking Forge…", false) {
            override fun run(indicator: ProgressIndicator) {
                // 1. Connect if needed
                if (!client.isConnected()) {
                    indicator.text = "Connecting to Forge daemon…"
                    val connectResult = client.connect(project.basePath ?: "")
                    if (connectResult.isFailure) {
                        showDaemonNotRunningNotification(project)
                        return
                    }
                }

                // 2. Ask
                indicator.text = "Waiting for Forge response…"
                val askResult = client.ask(
                    instruction = instruction,
                    selection   = selection,
                    filePath    = filePath
                )

                // 3. Show result
                askResult.fold(
                    onSuccess  = { response -> showResponseInToolWindow(project, instruction, response) },
                    onFailure  = { ex ->
                        if (isConnectionError(ex)) {
                            client.disconnect()
                            showDaemonNotRunningNotification(project)
                        } else {
                            showErrorNotification(project, ex.message ?: "Unknown error")
                        }
                    }
                )
            }
        }.queue()
    }

    // -------------------------------------------------------------------------
    // UI helpers (called from background thread → must use invokeLater / EDT)
    // -------------------------------------------------------------------------

    private fun showResponseInToolWindow(project: Project, question: String, response: String) {
        com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
            val tw = ToolWindowManager.getInstance(project).getToolWindow("Forge")
            tw?.activate(null)

            // Post message into the tool window content
            val factory = tw?.contentManager?.contents
                ?.mapNotNull { it.component as? dev.forgeapp.forge.ui.ForgePanel }
                ?.firstOrNull()

            if (factory != null) {
                factory.appendMessage("You: $question")
                factory.appendMessage("Forge: $response\n")
            } else {
                // Fallback: plain dialog
                Messages.showInfoMessage(project, response, "Forge Response")
            }
        }
    }

    private fun showDaemonNotRunningNotification(project: Project) {
        com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
            NotificationGroupManager.getInstance()
                .getNotificationGroup("Forge Notifications")
                .createNotification(
                    "Forge daemon not running",
                    "Start it with: forge start",
                    NotificationType.WARNING
                )
                .notify(project)
        }
    }

    private fun showErrorNotification(project: Project, message: String) {
        com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
            NotificationGroupManager.getInstance()
                .getNotificationGroup("Forge Notifications")
                .createNotification("Forge error", message, NotificationType.ERROR)
                .notify(project)
        }
    }

    private fun isConnectionError(ex: Throwable): Boolean =
        ex is java.io.IOException && (
            ex.message?.contains("refused", ignoreCase = true) == true ||
            ex.message?.contains("reach Forge daemon", ignoreCase = true) == true ||
            ex.message?.contains("connect", ignoreCase = true) == true
        )
}
