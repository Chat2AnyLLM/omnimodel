package tools

// Category groups tools by behavior and intended runtime usage.
type Category string

const (
	CategoryFilesystem Category = "filesystem"
	CategoryShell      Category = "shell"
	CategoryWeb        Category = "web"
	CategoryUtility    Category = "utility"
	CategoryTask       Category = "task"
	CategoryPlan       Category = "plan"
	CategoryScheduler  Category = "scheduler"
	CategoryWorktree   Category = "worktree"
	CategoryNotebook   Category = "notebook"
	CategoryMCP        Category = "mcp"
	CategoryBrowser    Category = "browser"
	CategorySpec       Category = "spec"
)
