## Lens: security

Look for injection (CWE-78, CWE-89, CWE-94), SSRF and host validation flaws
(CWE-918), path traversal (CWE-22), broken access control (CWE-284), secrets in
code (CWE-312), unsafe deserialization (CWE-502), and weak crypto (CWE-327).

Trace every credential, token, or secret the change handles to everywhere it
can surface: exception messages, exception causes and chained exceptions
(a sanitized message with the raw exception attached as its cause still
leaks when the throwable is logged), log lines, string representations, and
values passed to user callbacks whose errors may be logged (CWE-209,
CWE-532).
Start each finding title with the CWE ID and name the attack input in the
evidence.
