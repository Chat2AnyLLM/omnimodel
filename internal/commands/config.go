package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"

	"github.com/spf13/cobra"
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage external tool config files (Claude Code, Codex, etc.)",
}

func init() {
	ConfigCmd.AddCommand(configListCmd)
	ConfigCmd.AddCommand(configGetCmd)

	configSetCmd.Flags().StringP("file", "f", "", "Path to file to read content from")
	configSetCmd.Flags().Bool("stdin", false, "Read content from stdin")
	ConfigCmd.AddCommand(configSetCmd)

	configImportCmd.Flags().StringP("file", "f", "", "Path to file to import (required)")
	_ = configImportCmd.MarkFlagRequired("file")
	ConfigCmd.AddCommand(configImportCmd)

	ConfigCmd.AddCommand(configBackupCmd)
}

// ─── list ─────────────────────────────────────────────────────────────────────

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available config files",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/config")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		configs, _ := resp["configs"].([]interface{})
		if len(configs) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "config files available")
		}

		table := NewTable("NAME", "LABEL", "EXISTS")
		for _, item := range configs {
			cfg, _ := item.(map[string]interface{})
			name, _ := cfg["name"].(string)
			label, _ := cfg["label"].(string)
			exists := "no"
			if v, ok := cfg["exists"].(bool); ok && v {
				exists = "yes"
			}
			table.AddRow(name, label, exists)
		}
		return table.Render(cmd.OutOrStdout())
	},
}

// ─── get ──────────────────────────────────────────────────────────────────────

var configGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Print the contents of a config file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/config/" + args[0])
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		out := cmd.OutOrStdout()
		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err != nil {
			fmt.Fprintln(out, string(data))
			return nil
		}
		if exists, ok := resp["exists"].(bool); ok && !exists {
			msg, _ := resp["message"].(string)
			fmt.Fprintf(out, "(file does not exist yet: %s)\n", msg)
			return nil
		}
		content, _ := resp["content"].(string)
		fmt.Fprint(out, content)
		return nil
	},
}

// ─── set ──────────────────────────────────────────────────────────────────────

var configSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Write content to a config file (from --file or --stdin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		fromStdin, _ := cmd.Flags().GetBool("stdin")

		var content string
		switch {
		case filePath != "":
			b, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}
			content = string(b)
		case fromStdin:
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			content = string(b)
		default:
			return fmt.Errorf("provide --file <path> or --stdin")
		}

		c := NewClient(cmd)
		body := map[string]string{"content": content}
		data, err := c.Put("/api/admin/config/"+args[0], body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Config '%s' saved.", args[0])
		return nil
	},
}

// ─── import ───────────────────────────────────────────────────────────────────

var configImportCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "Import a config file via file upload",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")

		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		// Build multipart form
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, err := w.CreateFormFile("file", filePath)
		if err != nil {
			return err
		}
		if _, err := fw.Write(fileContent); err != nil {
			return err
		}
		w.Close()

		c := NewClient(cmd)
		data, err := c.DoRaw("POST", "/api/admin/config/"+args[0]+"/import", w.FormDataContentType(), &buf)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Config '%s' imported from %s.", args[0], filePath)
		return nil
	},
}

// ─── backup ───────────────────────────────────────────────────────────────────

var configBackupCmd = &cobra.Command{
	Use:   "backup <name>",
	Short: "Create a timestamped backup of a config file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/config/"+args[0]+"/backup", nil)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err == nil {
			if backup, ok := resp["backup"].(string); ok {
				SuccessMsg(cmd, "Backup saved to %s", backup)
				return nil
			}
		}
		SuccessMsg(cmd, "Config '%s' backed up.", args[0])
		return nil
	},
}