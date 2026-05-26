const vscode = require('vscode');

/**
 * Hawk AI VS Code Extension
 * 
 * Provides IDE integration for the hawk AI coding agent.
 * Communicates with the hawk daemon server via HTTP API.
 */

let daemonClient = null;

function activate(context) {
    const config = vscode.workspace.getConfiguration('hawk');
    const host = config.get('daemonHost', '127.0.0.1');
    const port = config.get('daemonPort', 4590);
    const apiKey = config.get('apiKey', '');

    daemonClient = new HawkDaemonClient(host, port, apiKey);

    // Register commands
    context.subscriptions.push(
        vscode.commands.registerCommand('hawk.chat', () => openChat(context)),
        vscode.commands.registerCommand('hawk.review', () => reviewChanges(context)),
        vscode.commands.registerCommand('hawk.explain', () => explainSelection(context)),
        vscode.commands.registerCommand('hawk.refactor', () => refactorSelection(context)),
        vscode.commands.registerCommand('hawk.test', () => generateTests(context)),
        vscode.commands.registerCommand('hawk.impact', () => analyzeImpact(context)),
        vscode.commands.registerCommand('hawk.commit', () => smartCommit(context)),
        vscode.commands.registerCommand('hawk.status', () => showStatus(context))
    );

    // Auto-review on save if enabled
    if (config.get('autoReview', true)) {
        context.subscriptions.push(
            vscode.workspace.onDidSaveTextDocument((doc) => {
                if (doc.uri.scheme === 'file') {
                    autoReviewOnSave(doc);
                }
            })
        );
    }

    // Status bar item
    const statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    statusBarItem.text = "$(comment-discussion) Hawk";
    statusBarItem.tooltip = "Open Hawk AI Chat";
    statusBarItem.command = 'hawk.chat';
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);

    console.log('Hawk AI extension activated');
}

async function openChat(context) {
    const panel = vscode.window.createWebviewPanel(
        'hawkChat',
        'Hawk AI Chat',
        vscode.ViewColumn.Beside,
        { enableScripts: true }
    );

    panel.webview.html = getChatHTML();

    panel.webview.onDidReceiveMessage(async (message) => {
        if (message.type === 'chat') {
            try {
                const response = await daemonClient.chat(message.text);
                panel.webview.postMessage({ type: 'response', text: response });
            } catch (err) {
                panel.webview.postMessage({ type: 'error', text: err.message });
            }
        }
    });
}

async function explainSelection(context) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) return;

    const selection = editor.document.getText(editor.selection);
    if (!selection) {
        vscode.window.showWarningMessage('Hawk: Select some code to explain');
        return;
    }

    const fileName = editor.document.fileName;
    const prompt = `Explain this code from ${fileName}:\n\`\`\`\n${selection}\n\`\`\``;

    try {
        vscode.window.showInformationMessage('Hawk: Analyzing code...');
        const response = await daemonClient.chat(prompt);
        
        const panel = vscode.window.createWebviewPanel(
            'hawkExplain',
            'Hawk: Code Explanation',
            vscode.ViewColumn.Beside,
            {}
        );
        panel.webview.html = `<html><body><pre>${escapeHtml(response)}</pre></body></html>`;
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function reviewChanges(context) {
    try {
        vscode.window.showInformationMessage('Hawk: Reviewing changes...');
        const response = await daemonClient.chat('Review the current git changes. Focus on correctness, security, and style.');
        
        const panel = vscode.window.createWebviewPanel(
            'hawkReview',
            'Hawk: Code Review',
            vscode.ViewColumn.Beside,
            {}
        );
        panel.webview.html = `<html><body><pre>${escapeHtml(response)}</pre></body></html>`;
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function refactorSelection(context) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) return;

    const selection = editor.document.getText(editor.selection);
    if (!selection) {
        vscode.window.showWarningMessage('Hawk: Select some code to refactor');
        return;
    }

    const fileName = editor.document.fileName;
    const prompt = `Suggest refactoring improvements for this code from ${fileName}:\n\`\`\`\n${selection}\n\`\`\`\nFocus on readability, performance, and maintainability.`;

    try {
        vscode.window.showInformationMessage('Hawk: Analyzing for refactoring...');
        const response = await daemonClient.chat(prompt);
        
        const panel = vscode.window.createWebviewPanel(
            'hawkRefactor',
            'Hawk: Refactoring Suggestions',
            vscode.ViewColumn.Beside,
            {}
        );
        panel.webview.html = `<html><body><pre>${escapeHtml(response)}</pre></body></html>`;
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function generateTests(context) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) return;

    const fileName = editor.document.fileName;
    const content = editor.document.getText();
    const prompt = `Generate tests for ${fileName}. Here's the current content:\n\`\`\`\n${content}\n\`\`\``;

    try {
        vscode.window.showInformationMessage('Hawk: Generating tests...');
        const response = await daemonClient.chat(prompt);
        
        const panel = vscode.window.createWebviewPanel(
            'hawkTests',
            'Hawk: Generated Tests',
            vscode.ViewColumn.Beside,
            {}
        );
        panel.webview.html = `<html><body><pre>${escapeHtml(response)}</pre></body></html>`;
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function analyzeImpact(context) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) return;

    const fileName = editor.document.fileName;
    const prompt = `Analyze the cross-file impact of changes to ${fileName}. What files are affected? What tests should be run?`;

    try {
        vscode.window.showInformationMessage('Hawk: Analyzing impact...');
        const response = await daemonClient.chat(prompt);
        
        const panel = vscode.window.createWebviewPanel(
            'hawkImpact',
            'Hawk: Impact Analysis',
            vscode.ViewColumn.Beside,
            {}
        );
        panel.webview.html = `<html><body><pre>${escapeHtml(response)}</pre></body></html>`;
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function smartCommit(context) {
    try {
        vscode.window.showInformationMessage('Hawk: Creating smart commit...');
        const response = await daemonClient.chat('Create a smart commit message for the staged changes. Follow conventional commits format.');
        vscode.window.showInformationMessage(`Hawk: ${response}`);
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function showStatus(context) {
    try {
        const health = await daemonClient.health();
        vscode.window.showInformationMessage(`Hawk: ${health}`);
    } catch (err) {
        vscode.window.showErrorMessage(`Hawk: ${err.message}`);
    }
}

async function autoReviewOnSave(doc) {
    // Only review code files
    const codeExtensions = ['.go', '.py', '.ts', '.js', '.rs', '.tsx', '.jsx'];
    const ext = doc.fileName.substring(doc.fileName.lastIndexOf('.'));
    if (!codeExtensions.includes(ext)) return;

    // Don't spam reviews - debounce
    if (autoReviewOnSave._timeout) {
        clearTimeout(autoReviewOnSave._timeout);
    }
    autoReviewOnSave._timeout = setTimeout(async () => {
        try {
            const response = await daemonClient.chat(`Quick review of ${doc.fileName} - any issues?`);
            if (response && !response.includes('no issues') && !response.toLowerCase().includes('looks good')) {
                vscode.window.showInformationMessage(`Hawk review: ${response.substring(0, 100)}...`);
            }
        } catch (err) {
            // Silently fail - don't annoy user
        }
    }, 2000);
}

/**
 * HTTP client for the hawk daemon API.
 */
class HawkDaemonClient {
    constructor(host, port, apiKey) {
        this.baseUrl = `http://${host}:${port}`;
        this.apiKey = apiKey;
    }

    async chat(message) {
        const response = await fetch(`${this.baseUrl}/v1/chat`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...(this.apiKey ? { 'Authorization': `Bearer ${this.apiKey}` } : {})
            },
            body: JSON.stringify({ message })
        });

        if (!response.ok) {
            throw new Error(`Hawk daemon error: ${response.status}`);
        }

        const data = await response.json();
        return data.response || data.message || JSON.stringify(data);
    }

    async health() {
        const response = await fetch(`${this.baseUrl}/v1/health`);
        if (!response.ok) {
            throw new Error(`Hawk daemon not running`);
        }
        const data = await response.json();
        return `Daemon running on ${this.baseUrl}`;
    }
}

function getChatHTML() {
    return `<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: var(--vscode-font-family); padding: 10px; }
        #messages { height: 400px; overflow-y: auto; margin-bottom: 10px; }
        .message { padding: 8px; margin: 4px 0; border-radius: 4px; }
        .user { background: var(--vscode-input-background); }
        .assistant { background: var(--vscode-editor-background); }
        input { width: 100%; padding: 8px; }
    </style>
</head>
<body>
    <div id="messages"></div>
    <input type="text" id="input" placeholder="Ask hawk anything..." />
    <script>
        const vscode = acquireVsCodeApi();
        const input = document.getElementById('input');
        const messages = document.getElementById('messages');
        
        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && input.value.trim()) {
                const msg = input.value.trim();
                messages.innerHTML += '<div class="message user">' + msg + '</div>';
                vscode.postMessage({ type: 'chat', text: msg });
                input.value = '';
            }
        });
        
        window.addEventListener('message', (e) => {
            const data = e.data;
            if (data.type === 'response') {
                messages.innerHTML += '<div class="message assistant">' + data.text + '</div>';
            } else if (data.type === 'error') {
                messages.innerHTML += '<div class="message" style="color: red;">' + data.text + '</div>';
            }
            messages.scrollTop = messages.scrollHeight;
        });
    </script>
</body>
</html>`;
}

function escapeHtml(text) {
    return text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

function deactivate() {}

module.exports = { activate, deactivate };
