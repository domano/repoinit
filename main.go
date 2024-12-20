package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func main() {
	// Load .env file if it exists
	godotenv.Load()

	// Get GitHub token from env
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required (either in .env file or exported)")
	}

	// Get current directory name
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get current directory:", err)
	}
	repoName := filepath.Base(pwd)

	// Initialize GitHub client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Create repository
	repo := &github.Repository{
		Name:     github.String(repoName),
		Private:  github.Bool(false),
		AutoInit: github.Bool(false),
	}

	repo, resp, err := client.Repositories.Create(ctx, "", repo)
	if err != nil {
		if resp != nil && resp.StatusCode == 422 { // HTTP 422 Unprocessable Entity typically means repo exists
			// Get authenticated user
			user, _, err := client.Users.Get(ctx, "")
			if err != nil {
				log.Fatal("Failed to get user:", err)
			}

			// Try to get the existing repo
			repo, _, err = client.Repositories.Get(ctx, *user.Login, repoName)
			if err != nil {
				log.Fatal("Failed to get existing repository:", err)
			}
			fmt.Printf("Using existing repository: %s\n", *repo.HTMLURL)
		} else {
			log.Fatal("Failed to create repository:", err)
		}
	} else {
		fmt.Printf("Created repository: %s\n", *repo.HTMLURL)
	}

	// Initialize git repository locally if not already initialized
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		if err := execCmd("git", "init"); err != nil {
			log.Fatal("Failed to init git:", err)
		}
	}

	// Check if remote exists and remove it if it does
	removeCmd := exec.Command("git", "remote", "remove", "origin")
	removeCmd.Run() // ignore errors since remote might not exist

	// Add remote
	remoteURL := fmt.Sprintf("git@github.com:%s.git", *repo.FullName)
	if err := execCmd("git", "remote", "add", "origin", remoteURL); err != nil {
		log.Fatal("Failed to add remote:", err)
	}

	// Add .gitignore first if it exists
	if _, err := os.Stat(".gitignore"); err == nil {
		if err := execCmd("git", "add", ".gitignore"); err != nil {
			log.Printf("Warning: Failed to add .gitignore: %v", err)
		}
	}

	// Add all non-hidden files
	files, err := os.ReadDir(".")
	if err != nil {
		log.Fatal("Failed to read directory:", err)
	}

	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, ".") && !file.IsDir() && name != ".gitignore" {
			if err := execCmd("git", "add", name); err != nil {
				log.Printf("Warning: Failed to add %s: %v", name, err)
			}
		}
	}

	// Commit
	if err := execCmd("git", "commit", "-m", "Initial commit"); err != nil {
		log.Fatal("Failed to commit:", err)
	}

	// Get current branch name
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := branchCmd.Output()
	if err != nil {
		log.Fatal("Failed to get branch name:", err)
	}
	currentBranch := strings.TrimSpace(string(branchBytes))

	// Push
	if err := execCmd("git", "push", "-u", "origin", currentBranch); err != nil {
		log.Fatal("Failed to push:", err)
	}

	fmt.Println("Successfully initialized and pushed repository!")
}

func execCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
