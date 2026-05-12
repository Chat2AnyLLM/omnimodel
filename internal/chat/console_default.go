//go:build !windows

package chat

func configureUTF8Console() error {
	return nil
}
