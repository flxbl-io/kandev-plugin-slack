package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func conversationDir(kind string) (string, error) {
	root := os.Getenv("KANDEV_PLUGIN_DATA_DIR")
	if root == "" {
		return "", errors.New("conversation storage unavailable")
	}
	dir := filepath.Join(root, "conversations-v1", kind)
	return dir, os.MkdirAll(dir, 0700)
}
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func readConversation(kind, key string, out any) error {
	dir, err := conversationDir(kind)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// All records are fsynced before they become visible. Exclusive publication
// uses a hard link, so readers never observe an empty, partially-written claim.
func writeConversation(kind, key string, value any, exclusive bool) error {
	dir, err := conversationDir(kind)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	target := filepath.Join(dir, key+".json")
	if exclusive {
		err = os.Link(name, target)
	} else {
		err = os.Rename(name, target)
	}
	if err != nil {
		return err
	}
	return syncDir(dir)
}
func archiveConversation(key string) error {
	from, err := conversationDir("inbox")
	if err != nil {
		return err
	}
	to, err := conversationDir("done")
	if err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(from, key+".json"), filepath.Join(to, key+".json")); err != nil {
		return err
	}
	if err = syncDir(to); err != nil {
		return err
	}
	return syncDir(from)
}
