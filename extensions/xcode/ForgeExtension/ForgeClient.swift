import Foundation

// MARK: - Models

struct BuildError: Codable {
    var file: String
    var message: String
    var severity: String
    var line: Int
    var col: Int
}

private struct ConnectRequest: Encodable {
    let ide: String
    let project_path: String
}

private struct ConnectResponse: Decodable {
    let token: String
    let context_id: String
}

private struct AskRequest: Encodable {
    let context_id: String
    let instruction: String
    let selection: String?
    let file_path: String?
    let stream: Bool
}

private struct AskResponse: Decodable {
    let response: String
}

private struct ErrorsResponse: Decodable {
    let errors: [BuildError]
}

private struct HealthResponse: Decodable {
    let status: String
    let version: String
}

// MARK: - ForgeError

enum ForgeError: LocalizedError {
    case notConnected
    case invalidResponse
    case httpError(Int)
    case decodingError(String)
    case networkError(String)

    var errorDescription: String? {
        switch self {
        case .notConnected:
            return "Not connected to Forge daemon. Make sure `forge start` is running."
        case .invalidResponse:
            return "Invalid response from Forge daemon."
        case .httpError(let code):
            return "Forge daemon returned HTTP \(code)."
        case .decodingError(let detail):
            return "Could not parse daemon response: \(detail)"
        case .networkError(let detail):
            return "Network error: \(detail)"
        }
    }
}

// MARK: - ForgeClient

final class ForgeClient {

    static let shared = ForgeClient()

    private let baseURL = "http://localhost:7878"
    private let session: URLSession
    private let defaults = UserDefaults(suiteName: "dev.forgeapp.ForgeXcode") ?? .standard

    private var token: String? {
        get { defaults.string(forKey: "forge_token") }
        set { defaults.set(newValue, forKey: "forge_token") }
    }

    private var contextID: String? {
        get { defaults.string(forKey: "forge_context_id") }
        set { defaults.set(newValue, forKey: "forge_context_id") }
    }

    private init() {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 30
        config.timeoutIntervalForResource = 60
        session = URLSession(configuration: config)
    }

    // MARK: - Public API

    /// Connect to the daemon and store credentials.
    func connect(projectPath: String, completion: @escaping (Result<Void, Error>) -> Void) {
        guard let url = URL(string: "\(baseURL)/ide/v1/connect") else {
            completion(.failure(ForgeError.invalidResponse))
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let body = ConnectRequest(ide: "xcode", project_path: projectPath)
        do {
            request.httpBody = try JSONEncoder().encode(body)
        } catch {
            completion(.failure(error))
            return
        }

        session.dataTask(with: request) { [weak self] data, response, error in
            guard let self else { return }

            if let error {
                completion(.failure(ForgeError.networkError(error.localizedDescription)))
                return
            }

            guard let http = response as? HTTPURLResponse else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            guard (200...299).contains(http.statusCode) else {
                completion(.failure(ForgeError.httpError(http.statusCode)))
                return
            }

            guard let data else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            do {
                let parsed = try JSONDecoder().decode(ConnectResponse.self, from: data)
                self.token = parsed.token
                self.contextID = parsed.context_id
                completion(.success(()))
            } catch {
                completion(.failure(ForgeError.decodingError(error.localizedDescription)))
            }
        }.resume()
    }

    /// Ask Claude a question with optional code selection and file context.
    func ask(
        instruction: String,
        selection: String?,
        filePath: String?,
        completion: @escaping (Result<String, Error>) -> Void
    ) {
        guard let token, let contextID else {
            completion(.failure(ForgeError.notConnected))
            return
        }

        guard let url = URL(string: "\(baseURL)/ide/v1/ask") else {
            completion(.failure(ForgeError.invalidResponse))
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(token, forHTTPHeaderField: "X-Forge-Token")

        let body = AskRequest(
            context_id: contextID,
            instruction: instruction,
            selection: selection,
            file_path: filePath,
            stream: false
        )

        do {
            request.httpBody = try JSONEncoder().encode(body)
        } catch {
            completion(.failure(error))
            return
        }

        session.dataTask(with: request) { data, response, error in
            if let error {
                completion(.failure(ForgeError.networkError(error.localizedDescription)))
                return
            }

            guard let http = response as? HTTPURLResponse else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            guard (200...299).contains(http.statusCode) else {
                completion(.failure(ForgeError.httpError(http.statusCode)))
                return
            }

            guard let data else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            do {
                let parsed = try JSONDecoder().decode(AskResponse.self, from: data)
                completion(.success(parsed.response))
            } catch {
                // Maybe the daemon returned plain text
                if let text = String(data: data, encoding: .utf8), !text.isEmpty {
                    completion(.success(text))
                } else {
                    completion(.failure(ForgeError.decodingError(error.localizedDescription)))
                }
            }
        }.resume()
    }

    /// Fetch current build errors from the daemon.
    func getErrors(completion: @escaping (Result<[BuildError], Error>) -> Void) {
        guard let token else {
            completion(.failure(ForgeError.notConnected))
            return
        }

        guard let url = URL(string: "\(baseURL)/ide/v1/errors") else {
            completion(.failure(ForgeError.invalidResponse))
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.setValue(token, forHTTPHeaderField: "X-Forge-Token")

        session.dataTask(with: request) { data, response, error in
            if let error {
                completion(.failure(ForgeError.networkError(error.localizedDescription)))
                return
            }

            guard let http = response as? HTTPURLResponse else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            guard (200...299).contains(http.statusCode) else {
                completion(.failure(ForgeError.httpError(http.statusCode)))
                return
            }

            guard let data else {
                completion(.failure(ForgeError.invalidResponse))
                return
            }

            do {
                let parsed = try JSONDecoder().decode(ErrorsResponse.self, from: data)
                completion(.success(parsed.errors))
            } catch {
                completion(.failure(ForgeError.decodingError(error.localizedDescription)))
            }
        }.resume()
    }

    /// Ping the daemon health endpoint.
    func checkHealth(completion: @escaping (Bool) -> Void) {
        guard let url = URL(string: "\(baseURL)/health") else {
            completion(false)
            return
        }

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 5

        session.dataTask(with: request) { data, response, _ in
            guard
                let http = response as? HTTPURLResponse,
                (200...299).contains(http.statusCode),
                let data,
                let parsed = try? JSONDecoder().decode(HealthResponse.self, from: data),
                parsed.status == "ok"
            else {
                completion(false)
                return
            }
            completion(true)
        }.resume()
    }

    /// True when we have a stored token and context ID.
    var isConnected: Bool { token != nil && contextID != nil }

    /// Ensure connected, connecting first if needed.
    func ensureConnected(projectPath: String, completion: @escaping (Result<Void, Error>) -> Void) {
        if isConnected {
            completion(.success(()))
        } else {
            connect(projectPath: projectPath, completion: completion)
        }
    }
}
