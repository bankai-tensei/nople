// Package shellreg puts Nople into the shell of the OS for the current user:
// "Open with Nople" on the context menu of folders, of the inside of a
// folder, of drives and of files, and Nople among the programs that open the
// text and source files of Extensions, so that the OS lets the user make it
// the one that opens them. Windows does not let a program make itself the
// default; the user picks it in Settings → Apps → Default apps, or in "Open
// with". Each extension gets a ProgID and an icon of its own, so that its
// files keep an icon of their type once Nople opens them, as they do with
// VS Code. Only Windows has this; elsewhere Register and Unregister fail.
package shellreg

// Labels are the words the shell shows, in the language of the UI.
type Labels struct {
	// Open is the item of the context menu, such as "Open with Nople".
	Open string
	// Description says what Nople is, in Default apps.
	Description string
	// TypeName names the type of the files of an extension, such as "GO
	// File" for ".go", which Explorer shows once Nople opens them.
	TypeName func(ext string) string
}

// Extensions are the extensions Nople offers to open, with their dot.
var Extensions = []string{
	".txt", ".md", ".markdown", ".log", ".csv", ".tsv",
	".json", ".jsonc", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env", ".properties", ".xml",
	".go", ".mod", ".sum",
	".py", ".rb", ".php", ".lua", ".pl", ".r",
	".js", ".mjs", ".cjs", ".ts", ".mts", ".cts", ".jsx", ".tsx", ".vue", ".svelte",
	".html", ".htm", ".css", ".scss", ".sass", ".less",
	".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".cs", ".java", ".kt", ".kts", ".scala", ".swift", ".dart", ".rs", ".zig",
	".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".bat", ".cmd",
	".sql", ".graphql", ".proto", ".gradle", ".cmake", ".mk",
	".gitignore", ".gitattributes", ".editorconfig",
}
