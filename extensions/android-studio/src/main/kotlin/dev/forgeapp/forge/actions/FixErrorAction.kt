package dev.forgeapp.forge.actions

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import dev.forgeapp.forge.api.BuildError
import dev.forgeapp.forge.api.ForgeClient

/**
 * Right-click → Forge → Fix Current Error
 *
 * Fetches errors from the daemon, picks one matching the current file
 * (or lets the user choose), then asks Forge to fix it and applies the
 * suggested replacement text via a WriteCommandAction.
 */
class FixErrorAction : AnAction() {

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabled = e.project != null && e.getData(CommonDataKeys.EDITOR) != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project  = e.project ?: return
        val editor   = e.getData(CommonDataKeys.EDITOR) ?: return
        val filePath = editor.virtualFile?.path

        val client = ForgeClient.instance

        object : Task.Backgroundable(project, "Fetching errors from Forge…", false) {
            override fun run(indicator: ProgressIndicator) {
                // 1. Connect if needed
                if (!client.isConnected()) {
                    indicator.text = "Connecting to Forge daemon…"
                    val cr = client.connect(project.basePath ?: "")
                    if (cr.isFailure) {
                        showNotification(
                            project,
                            "Forge daemon not running",
                            "Start it with: forge start",
                            NotificationType.WARNING
                        )
                        return
                    }
                }

                // 2. Fetch errors
                indicator.text = "Fetching build errors…"
                val errorsResult = client.getErrors()
                if (errorsResult.isFailure) {
                    showNotification(
                        project,
                        "Forge error",
                        errorsResult.exceptionOrNull()?.message ?: "Could not fetch errors",
                        NotificationType.ERROR
                    )
                    return
                }

                val allErrors = errorsResult.getOrDefault(emptyList())
                if (allErrors.isEmpty()) {
                    ApplicationManager.getApplication().invokeLater {
                        Messages.showInfoMessage(
                            project,
                            "No errors reported by the Forge daemon.",
                            "Forge — Fix Error"
                        )
                    }
                    return
                }

                // 3. Pick an error — prefer one in the current file
                val chosen = pickError(project, allErrors, filePath) ?: return

                // 4. Collect current file content as context for the fix
                val fileContent: String = readEditorText(editor)

                // 5. Ask Forge to fix it
                indicator.text = "Asking Forge to fix: ${chosen.message.take(60)}…"
                val fixResult = client.ask(
                    instruction = "Fix this error: ${chosen.message}",
                    selection   = fileContent,
                    filePath    = filePath
                )

                fixResult.fold(
                    onSuccess = { response ->
                        ApplicationManager.getApplication().invokeLater {
                            applyFix(project, editor, response)
                        }
                    },
                    onFailure = { ex ->
                        showNotification(
                            project,
                            "Forge error",
                            ex.message ?: "Unknown error",
                            NotificationType.ERROR
                        )
                    }
                )
            }
        }.queue()
    }

    // -------------------------------------------------------------------------
    // Error selection
    // -------------------------------------------------------------------------

    /**
     * Returns the error to fix.
     * - If there is exactly one error in the current file → use it immediately.
     * - Otherwise let the user pick from a list on the EDT (blocks the calling
     *   background thread via [ApplicationManager.getApplication().invokeAndWait]).
     */
    private fun pickError(
        project: Project,
        errors: List<BuildError>,
        currentFilePath: String?
    ): BuildError? {
        // Filter to errors in the current file
        val fileErrors = if (currentFilePath != null)
            errors.filter { it.file == currentFilePath || currentFilePath.endsWith(it.file) }
        else
            emptyList()

        // Exactly one matching error — no need to ask the user
        if (fileErrors.size == 1) return fileErrors.first()

        // Multiple (or zero matching) errors → show a chooser on the EDT
        var chosen: BuildError? = null
        val candidates = if (fileErrors.isNotEmpty()) fileErrors else errors

        ApplicationManager.getApplication().invokeAndWait {
            val descriptions = candidates.map { e ->
                "${e.file.substringAfterLast('/')}:${e.line} — ${e.message.take(80)}"
            }.toTypedArray()

            val idx = Messages.showChooseDialog(
                project,
                "Select the error to fix:",
                "Forge — Fix Error",
                Messages.getQuestionIcon(),
                descriptions,
                descriptions.firstOrNull()
            )

            if (idx >= 0) chosen = candidates[idx]
        }

        return chosen
    }

    // -------------------------------------------------------------------------
    // Apply the fix
    // -------------------------------------------------------------------------

    /**
     * Shows the proposed fix in a confirmation dialog.  If the user accepts,
     * replaces the entire document content with [fixText] inside a
     * [WriteCommandAction] so the change is undoable.
     *
     * The daemon returns free-form text that may include prose explanation as
     * well as a code block.  We extract the largest fenced code block if one
     * exists; otherwise we use the full response.
     */
    private fun applyFix(project: Project, editor: Editor, fixText: String) {
        val code = extractCodeBlock(fixText) ?: fixText.trim()

        val confirmed = Messages.showYesNoDialog(
            project,
            "Forge suggests the following fix:\n\n${code.take(500)}${if (code.length > 500) "\n…(truncated)" else ""}",
            "Forge — Apply Fix?",
            "Apply",
            "Cancel",
            Messages.getQuestionIcon()
        )

        if (confirmed != Messages.YES) return

        WriteCommandAction.runWriteCommandAction(project, "Forge Fix", null, {
            val doc = editor.document
            doc.replaceString(0, doc.textLength, code)
        })
    }

    // -------------------------------------------------------------------------
    // Utilities
    // -------------------------------------------------------------------------

    private fun readEditorText(editor: Editor): String {
        var text = ""
        ApplicationManager.getApplication().runReadAction {
            text = editor.document.text
        }
        return text
    }

    /** Extracts the content of the first fenced code block (``` … ```) if present. */
    private fun extractCodeBlock(text: String): String? {
        val fenceOpen  = text.indexOf("```")
        if (fenceOpen < 0) return null

        // Skip optional language hint on the opening fence line
        val afterFence = text.indexOf('\n', fenceOpen)
        if (afterFence < 0) return null

        val fenceClose = text.indexOf("```", afterFence)
        if (fenceClose < 0) return null

        return text.substring(afterFence + 1, fenceClose).trim()
    }

    private fun showNotification(
        project: Project,
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
}
