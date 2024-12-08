# repoinit 🚀

A simple CLI tool that initializes a GitHub repository from your current directory. It automatically creates the repo, sets up git, and pushes your files - all in one command!

## Features

- 🔄 Creates a GitHub repo with your current folder name
- 📁 Handles `.gitignore` files correctly
- 🔑 Interactive setup for GitHub token
- 🛡️ Works with both new and existing empty repositories
- 🌲 Respects your git branch naming preferences

## Installation

```bash
go install github.com/yourusername/repoinit@latest
```

Or clone and build:
```bash
git clone https://github.com/yourusername/repoinit.git
cd repoinit
go build
```

## First Run

Just run `repoinit` and follow the interactive setup! It will:
1. Guide you through creating a GitHub token
2. Save it securely in `~/.config/repoinit/token`
3. Create your repository

## Usage

Navigate to your project directory and run:
```bash
repoinit
```

That's it! Your code will be pushed to a new GitHub repository.

### Options

```bash
repoinit [flags]
  -private    Create a private repository (default: false)
  -name       Specify a custom repository name (default: current directory name)
```

### Configuration

Your GitHub token is stored in `~/.config/repoinit/token`. To update it, simply delete this file and run `repoinit` again.

## Common Issues

- **"Invalid token"**: Delete `~/.config/repoinit/token` and run repoinit again to set up a new token
- **"Repository exists"**: The tool will try to use the existing repo if it's empty
- **Branch name mismatch**: Set your default branch name with `git config --global init.defaultBranch main`

## Contributing

Pull requests are welcome! Feel free to:
- 🐛 Report bugs
- 💡 Suggest features
- 🔧 Submit PRs

## License

MIT - do whatever you want! 🎉
