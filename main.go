package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "github.com/google/go-github/v57/github"
    "github.com/joho/godotenv"
    "golang.org/x/oauth2"
)

func main() {
	// Load .env file if it exists
	godotenv.Load()

    // Resolve GitHub token via env, config file, gh CLI, or OAuth device flow
    ctx := context.Background()
    token, err := resolveGitHubToken(ctx)
    if err != nil || token == "" {
        log.Fatalf("Authentication required. %v", err)
    }

	// Get current directory name
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get current directory:", err)
	}
	repoName := filepath.Base(pwd)

    // Initialize GitHub client
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

// resolveGitHubToken attempts to find or obtain a GitHub token in the following order:
// 1) GITHUB_TOKEN env var
// 2) token stored at ~/.config/repoinit/token
// 3) gh CLI (gh auth token or gh auth login --web)
// 4) OAuth Device Flow using GITHUB_OAUTH_CLIENT_ID
func resolveGitHubToken(ctx context.Context) (string, error) {
    // 1) env var
    envToken := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
    if envToken != "" {
        return envToken, nil
    }

    // 2) config file
    if token, _ := readStoredToken(); token != "" {
        return token, nil
    }

    // 3) gh CLI
    if token, err := tryGhToken(); err == nil && token != "" {
        // Persist for next time
        _ = writeStoredToken(token)
        return token, nil
    } else {
        // Attempt interactive gh login if available
        if err := tryGhWebLogin(); err == nil {
            if token, err := tryGhToken(); err == nil && token != "" {
                _ = writeStoredToken(token)
                return token, nil
            }
        }
    }

    // 4) OAuth Device Flow
    clientID := strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CLIENT_ID"))
    if clientID != "" {
        token, err := runDeviceFlow(ctx, clientID, []string{"repo"})
        if err != nil {
            return "", err
        }
        if token != "" {
            _ = writeStoredToken(token)
            return token, nil
        }
    }

    return "", errors.New("no token found. Set GITHUB_TOKEN, or install GitHub CLI (gh) to login via web, or set GITHUB_OAUTH_CLIENT_ID to use device OAuth. See https://docs.github.com/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps for details.")
}

func configTokenPath() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    path := filepath.Join(dir, "repoinit", "token")
    return path, nil
}

func readStoredToken() (string, error) {
    path, err := configTokenPath()
    if err != nil {
        return "", err
    }
    data, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(data)), nil
}

func writeStoredToken(token string) error {
    path, err := configTokenPath()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
        return err
    }
    return os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), 0o600)
}

func tryGhToken() (string, error) {
    if _, err := exec.LookPath("gh"); err != nil {
        return "", err
    }
    cmd := exec.Command("gh", "auth", "token")
    out, err := cmd.Output()
    if err != nil {
        return "", err
    }
    token := strings.TrimSpace(string(out))
    if token == "" {
        return "", errors.New("empty gh token")
    }
    return token, nil
}

func tryGhWebLogin() error {
    if _, err := exec.LookPath("gh"); err != nil {
        return err
    }
    // Request repo scope to create repositories
    cmd := exec.Command("gh", "auth", "login", "--web", "--scopes", "repo")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    return cmd.Run()
}

// Device flow responses
type deviceCodeResponse struct {
    DeviceCode              string `json:"device_code"`
    UserCode                string `json:"user_code"`
    VerificationURI         string `json:"verification_uri"`
    VerificationURIComplete string `json:"verification_uri_complete"`
    ExpiresIn               int    `json:"expires_in"`
    Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType   string `json:"token_type"`
    Scope       string `json:"scope"`
    Error       string `json:"error"`
    ErrorDesc   string `json:"error_description"`
}

// runDeviceFlow implements GitHub's OAuth Device Authorization Grant
// Docs: https://docs.github.com/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow
func runDeviceFlow(ctx context.Context, clientID string, scopes []string) (string, error) {
    // 1) Initiate device code
    values := url.Values{}
    values.Set("client_id", clientID)
    values.Set("scope", strings.Join(scopes, ","))

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/device/code", strings.NewReader(values.Encode()))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("device code request failed: %s", strings.TrimSpace(string(body)))
    }

    var dc deviceCodeResponse
    if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
        return "", err
    }

    // Present link to user
    fmt.Println("To authenticate with GitHub, open this link in your browser:")
    if dc.VerificationURIComplete != "" {
        fmt.Printf("  %s\n", dc.VerificationURIComplete)
    } else {
        fmt.Printf("  %s\n", dc.VerificationURI)
        fmt.Printf("and enter the code: %s\n", dc.UserCode)
    }

    // 2) Poll for token
    pollInterval := time.Duration(dc.Interval)
    if pollInterval <= 0 {
        pollInterval = 5
    }
    ticker := time.NewTicker(pollInterval * time.Second)
    defer ticker.Stop()
    timeout := time.After(time.Duration(dc.ExpiresIn) * time.Second)

    for {
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        case <-timeout:
            return "", errors.New("device code expired; please try again")
        case <-ticker.C:
            token, cont, err := pollDeviceToken(ctx, clientID, dc.DeviceCode)
            if err != nil {
                return "", err
            }
            if token != "" {
                return token, nil
            }
            if !cont {
                return "", errors.New("authorization declined")
            }
        }
    }
}

func pollDeviceToken(ctx context.Context, clientID, deviceCode string) (token string, continuePolling bool, err error) {
    values := url.Values{}
    values.Set("client_id", clientID)
    values.Set("device_code", deviceCode)
    values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(values.Encode()))
    if err != nil {
        return "", true, err
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", true, err
    }
    defer resp.Body.Close()
    var tr deviceTokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
        return "", true, err
    }
    switch tr.Error {
    case "":
        return strings.TrimSpace(tr.AccessToken), false, nil
    case "authorization_pending":
        return "", true, nil
    case "slow_down":
        // Caller keeps same interval; next tick will be later
        return "", true, nil
    case "expired_token":
        return "", false, errors.New("device code expired")
    case "access_denied":
        return "", false, errors.New("access denied by user")
    default:
        return "", false, fmt.Errorf("oauth error: %s", tr.Error)
    }
}
