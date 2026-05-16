package dev.forgeapp.forge.settings

import com.intellij.openapi.options.Configurable
import com.intellij.openapi.ui.Messages
import dev.forgeapp.forge.api.ForgeClient
import java.awt.BorderLayout
import java.awt.FlowLayout
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import javax.swing.*

/**
 * Settings panel shown under File → Settings → Tools → Forge AI.
 */
class ForgeConfigurable : Configurable {

    // -------------------------------------------------------------------------
    // UI controls (created lazily in createComponent)
    // -------------------------------------------------------------------------
    private var panel: JPanel? = null
    private lateinit var portField: JTextField
    private lateinit var autoConnectCheckBox: JCheckBox
    private lateinit var statusLabel: JLabel

    // -------------------------------------------------------------------------
    // Configurable contract
    // -------------------------------------------------------------------------

    override fun getDisplayName(): String = "Forge AI"

    override fun createComponent(): JComponent {
        val settings = ForgeSettings.getInstance()

        // --- Controls --------------------------------------------------------
        portField = JTextField(settings.daemonPort.toString(), 8)

        autoConnectCheckBox = JCheckBox(
            "Auto-connect when a project is opened",
            settings.autoConnectOnOpen
        )

        statusLabel = JLabel(" ")

        val testButton = JButton("Test Connection").apply {
            addActionListener { testConnection() }
        }

        // --- Layout ----------------------------------------------------------
        val form = JPanel(GridBagLayout())
        val gbc = GridBagConstraints().apply {
            anchor = GridBagConstraints.WEST
            fill   = GridBagConstraints.HORIZONTAL
            insets = Insets(4, 4, 4, 4)
        }

        // Row 0 — port label
        gbc.gridx = 0; gbc.gridy = 0; gbc.weightx = 0.0
        form.add(JLabel("Daemon port:"), gbc)

        // Row 0 — port field
        gbc.gridx = 1; gbc.gridy = 0; gbc.weightx = 1.0
        form.add(portField, gbc)

        // Row 1 — auto-connect checkbox (spans 2 columns)
        gbc.gridx = 0; gbc.gridy = 1; gbc.gridwidth = 2; gbc.weightx = 1.0
        form.add(autoConnectCheckBox, gbc)
        gbc.gridwidth = 1

        // Row 2 — Test Connection button + status label
        val testRow = JPanel(FlowLayout(FlowLayout.LEFT, 0, 0))
        testRow.add(testButton)
        testRow.add(Box.createHorizontalStrut(12))
        testRow.add(statusLabel)

        gbc.gridx = 0; gbc.gridy = 2; gbc.gridwidth = 2; gbc.weightx = 1.0
        form.add(testRow, gbc)

        // Outer panel with padding and a vertical glue so the form sits at top
        val outer = JPanel(BorderLayout())
        outer.border = BorderFactory.createEmptyBorder(8, 8, 8, 8)
        outer.add(form, BorderLayout.NORTH)

        panel = outer
        return outer
    }

    override fun isModified(): Boolean {
        val settings = ForgeSettings.getInstance()
        val portOk = portField.text.trim().toIntOrNull() == settings.daemonPort
        val autoOk = autoConnectCheckBox.isSelected == settings.autoConnectOnOpen
        return !portOk || !autoOk
    }

    override fun apply() {
        val newPort = portField.text.trim().toIntOrNull()
        if (newPort == null || newPort !in 1..65535) {
            Messages.showErrorDialog(
                panel,
                "Port must be a number between 1 and 65535.",
                "Invalid Port"
            )
            return
        }

        val settings = ForgeSettings.getInstance()
        settings.daemonPort = newPort
        settings.autoConnectOnOpen = autoConnectCheckBox.isSelected
    }

    override fun reset() {
        val settings = ForgeSettings.getInstance()
        portField.text = settings.daemonPort.toString()
        autoConnectCheckBox.isSelected = settings.autoConnectOnOpen
        statusLabel.text = " "
    }

    // -------------------------------------------------------------------------
    // Test Connection
    // -------------------------------------------------------------------------

    private fun testConnection() {
        statusLabel.text = "Checking…"
        statusLabel.foreground = UIManager.getColor("Label.foreground")

        // Run in a background thread so the EDT is not blocked
        Thread {
            val result = ForgeClient.instance.checkHealth()
            SwingUtilities.invokeLater {
                result.fold(
                    onSuccess = { health ->
                        statusLabel.text = "Connected — daemon ${health.version} (${health.status})"
                        statusLabel.foreground = java.awt.Color(0x2E, 0x7D, 0x32) // green-800
                    },
                    onFailure = { ex ->
                        statusLabel.text = "Failed: ${ex.message}"
                        statusLabel.foreground = java.awt.Color(0xC6, 0x28, 0x28) // red-800
                    }
                )
            }
        }.also { it.isDaemon = true }.start()
    }
}
