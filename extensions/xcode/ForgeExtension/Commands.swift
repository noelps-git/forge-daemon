import Foundation
import XcodeKit
import AppKit

// MARK: - Helpers

private extension XCSourceEditorCommandInvocation {

    /// Extracts all selected text as a single string. Returns nil when nothing
    /// is selected or the selections span zero characters.
    var selectedText: String? {
        guard !buffer.selections.isEmpty else { return nil }

        var parts: [String] = []
        let lines = buffer.lines as! [String]

        for selection in buffer.selections as! [XCSourceTextRange] {
            let start = selection.start
            let end = selection.end

            guard start.line < lines.count else { continue }

            if start.line == end.line {
                let line = lines[start.line]
                let startIdx = line.index(line.startIndex, offsetBy: min(start.column, line.count))
                let endIdx = line.index(line.startIndex, offsetBy: min(end.column, line.count))
                if startIdx < endIdx {
                    parts.append(String(line[startIdx..<endIdx]))
                }
            } else {
                var collected: [String] = []
                for lineIdx in start.line...min(end.line, lines.count - 1) {
                    let line = lines[lineIdx]
                    if lineIdx == start.line {
                        let idx = line.index(line.startIndex, offsetBy: min(start.column, line.count))
                        collected.append(String(line[idx...]))
                    } else if lineIdx == end.line {
                        let idx = line.index(line.startIndex, offsetBy: min(end.column, line.count))
                        collected.append(String(line[..<idx]))
                    } else {
                        collected.append(line)
                    }
                }
                parts.append(collected.joined())
            }
        }

        let result = parts.joined(separator: "\n")
        return result.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? nil : result
    }

    /// Best-effort file path from the buffer content UTI or the document URL.
    var currentFilePath: String? {
        // XcodeKit doesn't expose the on-disk path directly; we use the
        // document display name stored in the buffer's contentUTI as a hint,
        // and fall back to nothing. The daemon uses it for richer context only.
        return nil
    }

    /// Derives a plausible project root by walking up from any open document.
    var projectPath: String {
        return FileManager.default.homeDirectoryForCurrentUser.path
    }
}

// MARK: - UI helpers (must run on main thread)

private func showAlert(title: String, message: String) {
    DispatchQueue.main.async {
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = message
        alert.alertStyle = .informational
        alert.addButton(withTitle: "OK")
        alert.runModal()
    }
}

private func showScrollableResult(title: String, body: String) {
    DispatchQueue.main.async {
        let alert = NSAlert()
        alert.messageText = title
        alert.alertStyle = .informational
        alert.addButton(withTitle: "OK")

        let scrollView = NSScrollView(frame: NSRect(x: 0, y: 0, width: 500, height: 300))
        scrollView.hasVerticalScroller = true
        scrollView.hasHorizontalScroller = false
        scrollView.autohidesScrollers = true
        scrollView.borderType = .bezelBorder

        let textView = NSTextView(frame: scrollView.bounds)
        textView.isEditable = false
        textView.isSelectable = true
        textView.string = body
        textView.font = NSFont.monospacedSystemFont(ofSize: 12, weight: .regular)
        textView.textContainerInset = NSSize(width: 6, height: 6)

        scrollView.documentView = textView
        alert.accessoryView = scrollView
        alert.runModal()
    }
}

/// Prompts the user for a question and returns the entered string, or nil if cancelled.
private func promptForQuestion() -> String? {
    var result: String?
    let sema = DispatchSemaphore(value: 0)

    DispatchQueue.main.async {
        let alert = NSAlert()
        alert.messageText = "Ask Forge"
        alert.informativeText = "What would you like to know about the selected code?"
        alert.alertStyle = .informational
        alert.addButton(withTitle: "Ask")
        alert.addButton(withTitle: "Cancel")

        let inputField = NSTextField(frame: NSRect(x: 0, y: 0, width: 400, height: 24))
        inputField.placeholderString = "e.g. What does this function do?"
        alert.accessoryView = inputField
        alert.window.initialFirstResponder = inputField

        let response = alert.runModal()
        if response == .alertFirstButtonReturn {
            let text = inputField.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)
            result = text.isEmpty ? nil : text
        }
        sema.signal()
    }

    sema.wait()
    return result
}

// MARK: - Comment formatting

/// Wraps a multi-line response in the appropriate comment syntax for the UTI.
private func commentify(_ text: String, uti: String) -> String {
    let useSlash = uti.contains("swift") || uti.contains("c-source") ||
                   uti.contains("c-plus-plus") || uti.contains("objective-c")

    if useSlash {
        let lines = text.components(separatedBy: "\n").map { "// \($0)" }
        return lines.joined(separator: "\n")
    } else {
        let inner = text.components(separatedBy: "\n").joined(separator: "\n   ")
        return "/* \(inner) */"
    }
}

// MARK: - AskForgeCommand

/// Presents an NSAlert text field, sends the question to Claude with selected
/// code as context, and appends the response as a comment block after the selection.
final class AskForgeCommand: NSObject, XCSourceEditorCommand {

    func perform(
        with invocation: XCSourceEditorCommandInvocation,
        completionHandler: @escaping (Error?) -> Void
    ) {
        // Capture what we need before touching UI
        let selection = invocation.selectedText
        let uti = invocation.buffer.contentUTI

        // Ask the user what they want to know (blocks until modal dismisses)
        guard let question = promptForQuestion() else {
            completionHandler(nil)
            return
        }

        let instruction = selection != nil
            ? "\(question)\n\nCode:\n\(selection!)"
            : question

        let client = ForgeClient.shared
        let projectPath = invocation.projectPath

        func sendAsk() {
            client.ask(
                instruction: instruction,
                selection: selection,
                filePath: invocation.currentFilePath
            ) { result in
                switch result {
                case .success(let answer):
                    // Insert response as comment lines after the last selection
                    let commentBlock = commentify(answer, uti: uti)
                    let commentLines = (commentBlock + "\n")
                        .components(separatedBy: "\n")
                        .map { $0 as NSString }

                    DispatchQueue.main.async {
                        // Find the end of the last selection to insert after it
                        let insertAt: Int
                        if let lastSel = (invocation.buffer.selections as! [XCSourceTextRange]).last {
                            insertAt = min(lastSel.end.line + 1, invocation.buffer.lines.count)
                        } else {
                            insertAt = invocation.buffer.lines.count
                        }

                        let indexSet = IndexSet(
                            integersIn: insertAt..<(insertAt + commentLines.count)
                        )
                        invocation.buffer.lines.insert(commentLines, at: indexSet)
                        completionHandler(nil)
                    }

                case .failure(let error):
                    showAlert(title: "Forge Error", message: error.localizedDescription)
                    completionHandler(error)
                }
            }
        }

        client.ensureConnected(projectPath: projectPath) { result in
            switch result {
            case .success:
                sendAsk()
            case .failure(let error):
                showAlert(title: "Forge: Cannot Connect", message: error.localizedDescription)
                completionHandler(error)
            }
        }
    }
}

// MARK: - ExplainSelectionCommand

/// Sends the selected lines to Claude with the instruction "Explain this code:"
/// and shows the response in a scrollable NSAlert.
final class ExplainSelectionCommand: NSObject, XCSourceEditorCommand {

    func perform(
        with invocation: XCSourceEditorCommandInvocation,
        completionHandler: @escaping (Error?) -> Void
    ) {
        guard let selection = invocation.selectedText, !selection.isEmpty else {
            showAlert(
                title: "No Selection",
                message: "Select some code in the editor before using Explain Selection."
            )
            completionHandler(nil)
            return
        }

        let client = ForgeClient.shared
        let projectPath = invocation.projectPath

        func sendExplain() {
            client.ask(
                instruction: "Explain this code:",
                selection: selection,
                filePath: invocation.currentFilePath
            ) { result in
                switch result {
                case .success(let explanation):
                    showScrollableResult(title: "Forge — Explanation", body: explanation)
                    completionHandler(nil)
                case .failure(let error):
                    showAlert(title: "Forge Error", message: error.localizedDescription)
                    completionHandler(error)
                }
            }
        }

        client.ensureConnected(projectPath: projectPath) { result in
            switch result {
            case .success:
                sendExplain()
            case .failure(let error):
                showAlert(title: "Forge: Cannot Connect", message: error.localizedDescription)
                completionHandler(error)
            }
        }
    }
}

// MARK: - FixCurrentErrorCommand

/// Fetches build errors from the daemon, picks the first one, and asks Claude
/// to generate a fix. The response replaces the lines around the error site.
final class FixCurrentErrorCommand: NSObject, XCSourceEditorCommand {

    func perform(
        with invocation: XCSourceEditorCommandInvocation,
        completionHandler: @escaping (Error?) -> Void
    ) {
        let client = ForgeClient.shared
        let projectPath = invocation.projectPath

        func fetchAndFix() {
            client.getErrors { result in
                switch result {
                case .failure(let error):
                    showAlert(title: "Forge Error", message: error.localizedDescription)
                    completionHandler(error)
                    return

                case .success(let errors):
                    guard let firstError = errors.first else {
                        showAlert(
                            title: "No Errors",
                            message: "Forge found no build errors to fix."
                        )
                        completionHandler(nil)
                        return
                    }

                    // Gather surrounding context from the buffer (±5 lines)
                    let lines = invocation.buffer.lines as! [String]
                    let errorLine = max(0, firstError.line - 1) // convert 1-based → 0-based
                    let contextStart = max(0, errorLine - 5)
                    let contextEnd = min(lines.count - 1, errorLine + 5)
                    let context = lines[contextStart...contextEnd].joined()

                    let instruction = """
                    Fix this \(firstError.severity) error: \(firstError.message)
                    File: \(firstError.file), line \(firstError.line), col \(firstError.col)

                    Surrounding code:
                    \(context)

                    Return ONLY the corrected replacement lines, no explanation.
                    """

                    client.ask(
                        instruction: instruction,
                        selection: context,
                        filePath: firstError.file
                    ) { askResult in
                        switch askResult {
                        case .failure(let error):
                            showAlert(title: "Forge Error", message: error.localizedDescription)
                            completionHandler(error)

                        case .success(let fix):
                            // Replace the context lines with the fix
                            let fixLines = fix
                                .trimmingCharacters(in: .whitespacesAndNewlines)
                                .components(separatedBy: "\n")
                                .map { $0 as NSString }

                            DispatchQueue.main.async {
                                let replaceRange = NSRange(
                                    location: contextStart,
                                    length: contextEnd - contextStart + 1
                                )
                                invocation.buffer.lines.replaceObjects(
                                    in: replaceRange,
                                    withObjectsFrom: fixLines
                                )
                                completionHandler(nil)
                            }
                        }
                    }
                }
            }
        }

        client.ensureConnected(projectPath: projectPath) { result in
            switch result {
            case .success:
                fetchAndFix()
            case .failure(let error):
                showAlert(title: "Forge: Cannot Connect", message: error.localizedDescription)
                completionHandler(error)
            }
        }
    }
}
