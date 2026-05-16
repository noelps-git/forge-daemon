package dev.forgeapp.forge.ui

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory
import dev.forgeapp.forge.api.ForgeClient
import java.awt.BorderLayout
import java.awt.Color
import java.awt.Dimension
import java.awt.FlowLayout
import java.awt.Font
import java.awt.event.KeyAdapter
import java.awt.event.KeyEvent
import javax.swing.*
import javax.swing.border.EmptyBorder

// ---------------------------------------------------------------------------
// ToolWindowFactory — registered in plugin.xml
// ---------------------------------------------------------------------------

class ForgeToolWindowFactory : ToolWindowFactory {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val panel = ForgePanel(project)
        val content = ContentFactory.getInstance()
            .createContent(panel, "", false)
        toolWindow.contentManager.addContent(content)
    }
}

// ---------------------------------------------------------------------------
// ForgePanel — the actual Swing component placed inside the tool window
// ---------------------------------------------------------------------------

class ForgePanel(private val project: Project) : JPanel(BorderLayout()) {

    // -----------------------------------------------------------------------
    // UI controls
    // -----------------------------------------------------------------------
    private val conversationArea = JTextArea().apply {
        isEditable  = false
        lineWrap    = true
        wrapStyleWord = true
        font        = Font(Font.MONOSPACED, Font.PLAIN, 12)
        background  = UIManager.getColor("TextArea.background") ?: Color(0xF5, 0xF5, 0xF5)
        border      = EmptyBorder(8, 8, 8, 8)
    }

    private val inputField = JTextField().apply {
        toolTipText = "Type your question and press Enter or click Send"
        font        = Font(Font.SANS_SERIF, Font.PLAIN, 13)
    }

    private val sendButton = JButton("Send")
    private val connectButton = JButton("Connect")
    private val statusLabel = JLabel("Not connected").apply {
        foreground  = Color(0xC6, 0x28, 0x28)   // red — not connected
        font        = Font(Font.SANS_SERIF, Font.ITALIC, 11)
        border      = EmptyBorder(0, 6, 0, 0)
    }

    // -----------------------------------------------------------------------
    // Init
    // -----------------------------------------------------------------------
    init {
        border = EmptyBorder(0, 0, 0, 0)
        buildUI()
        wireListeners()
        refreshConnectionStatus()
    }

    // -----------------------------------------------------------------------
    // Public API used by action classes
    // -----------------------------------------------------------------------

    fun appendMessage(text: String) {
        SwingUtilities.invokeLater {
            conversationArea.append("$text\n")
            // Auto-scroll to bottom
            conversationArea.caretPosition = conversationArea.document.length
        }
    }

    // -----------------------------------------------------------------------
    // Layout
    // -----------------------------------------------------------------------

    private fun buildUI() {
        // ── Top toolbar ──────────────────────────────────────────────────────
        val toolbar = JPanel(FlowLayout(FlowLayout.LEFT, 4, 4)).apply {
            add(connectButton)
            add(statusLabel)
        }
        add(toolbar, BorderLayout.NORTH)

        // ── Scrollable conversation area ────────────────────────────────────
        val scrollPane = JScrollPane(
            conversationArea,
            JScrollPane.VERTICAL_SCROLLBAR_AS_NEEDED,
            JScrollPane.HORIZONTAL_SCROLLBAR_NEVER
        ).apply {
            border = BorderFactory.createMatteBorder(1, 0, 1, 0,
                UIManager.getColor("Separator.foreground") ?: Color.LIGHT_GRAY)
        }
        add(scrollPane, BorderLayout.CENTER)

        // ── Bottom input row ─────────────────────────────────────────────────
        val inputPanel = JPanel(BorderLayout(4, 0)).apply {
            border = EmptyBorder(4, 4, 4, 4)
        }
        inputField.preferredSize = Dimension(0, 32)
        sendButton.preferredSize = Dimension(64, 32)
        inputPanel.add(inputField,  BorderLayout.CENTER)
        inputPanel.add(sendButton,  BorderLayout.EAST)
        add(inputPanel, BorderLayout.SOUTH)
    }

    // -----------------------------------------------------------------------
    // Event wiring
    // -----------------------------------------------------------------------

    private fun wireListeners() {
        // Send on button click
        sendButton.addActionListener { handleSend() }

        // Send on Enter key in input field
        inputField.addKeyListener(object : KeyAdapter() {
            override fun keyPressed(e: KeyEvent) {
                if (e.keyCode == KeyEvent.VK_ENTER) handleSend()
            }
        })

        // Connect button
        connectButton.addActionListener { handleConnect() }
    }

    // -----------------------------------------------------------------------
    // Actions
    // -----------------------------------------------------------------------

    private fun handleSend() {
        val text = inputField.text.trim()
        if (text.isBlank()) return
        inputField.text = ""

        // Make controls non-interactive while waiting
        setInputEnabled(false)
        appendMessage("You: $text")

        Thread {
            val client = ForgeClient.instance

            // Auto-connect if not already connected
            if (!client.isConnected()) {
                val cr = client.connect(project.basePath ?: "")
                if (cr.isFailure) {
                    appendMessage("⚠ Could not connect to Forge daemon. Start it with: forge start\n")
                    SwingUtilities.invokeLater { setInputEnabled(true) }
                    return@Thread
                }
                SwingUtilities.invokeLater { markConnected() }
            }

            val result = client.ask(instruction = text)
            result.fold(
                onSuccess = { response ->
                    appendMessage("Forge: $response\n")
                },
                onFailure = { ex ->
                    if (isConnectionError(ex)) {
                        client.disconnect()
                        SwingUtilities.invokeLater { markDisconnected() }
                        appendMessage("⚠ Lost connection to Forge daemon: ${ex.message}\n")
                    } else {
                        appendMessage("⚠ Error: ${ex.message}\n")
                    }
                }
            )

            SwingUtilities.invokeLater { setInputEnabled(true) }
        }.also { it.isDaemon = true }.start()
    }

    private fun handleConnect() {
        connectButton.isEnabled = false
        statusLabel.text = "Connecting…"

        Thread {
            val client = ForgeClient.instance
            val result = client.connect(project.basePath ?: "")
            SwingUtilities.invokeLater {
                connectButton.isEnabled = true
                if (result.isSuccess) {
                    markConnected()
                    appendMessage("✓ Connected to Forge daemon.\n")
                } else {
                    markDisconnected()
                    val msg = result.exceptionOrNull()?.message ?: "Unknown error"
                    appendMessage("✗ Connection failed: $msg\n")
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
        }.also { it.isDaemon = true }.start()
    }

    // -----------------------------------------------------------------------
    // Status helpers
    // -----------------------------------------------------------------------

    private fun refreshConnectionStatus() {
        if (ForgeClient.instance.isConnected()) markConnected() else markDisconnected()
    }

    private fun markConnected() {
        statusLabel.text      = "Connected"
        statusLabel.foreground = Color(0x2E, 0x7D, 0x32)   // green-800
        connectButton.text    = "Reconnect"
    }

    private fun markDisconnected() {
        statusLabel.text      = "Not connected"
        statusLabel.foreground = Color(0xC6, 0x28, 0x28)   // red-800
        connectButton.text    = "Connect"
    }

    private fun setInputEnabled(enabled: Boolean) {
        inputField.isEnabled = enabled
        sendButton.isEnabled = enabled
    }

    private fun isConnectionError(ex: Throwable): Boolean =
        ex is java.io.IOException && (
            ex.message?.contains("refused", ignoreCase = true) == true ||
            ex.message?.contains("reach Forge daemon", ignoreCase = true) == true
        )
}
