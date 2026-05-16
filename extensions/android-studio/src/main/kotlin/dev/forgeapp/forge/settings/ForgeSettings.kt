package dev.forgeapp.forge.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage

@State(
    name = "ForgeSettings",
    storages = [Storage("forge.xml")]
)
class ForgeSettings : PersistentStateComponent<ForgeSettings.State> {

    data class State(
        var daemonPort: Int = 7878,
        var defaultInstruction: String = "",
        var autoConnectOnOpen: Boolean = true,
        var showInlineHints: Boolean = false
    )

    private var state = State()

    override fun getState(): State = state

    override fun loadState(state: State) {
        this.state = state
    }

    // Convenience accessors so callers don't have to go through getState()
    var daemonPort: Int
        get() = state.daemonPort
        set(v) { state.daemonPort = v }

    var defaultInstruction: String
        get() = state.defaultInstruction
        set(v) { state.defaultInstruction = v }

    var autoConnectOnOpen: Boolean
        get() = state.autoConnectOnOpen
        set(v) { state.autoConnectOnOpen = v }

    var showInlineHints: Boolean
        get() = state.showInlineHints
        set(v) { state.showInlineHints = v }

    companion object {
        fun getInstance(): ForgeSettings =
            ApplicationManager.getApplication().getService(ForgeSettings::class.java)
    }
}
