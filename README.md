# Hello World with Fyne v2.7.0 - AppTabs with Modular GUI

A clean, modular Hello World application built with [Fyne](https://fyne.io/), a modern cross-platform GUI toolkit for Go.

## About Fyne

Fyne is a free and open source widget toolkit for creating graphical user interfaces in Go. It is designed to be simple, scalable, and accessible across different platforms (Linux, macOS, Windows, Android, iOS, and web).

## Project Structure

```
llm/
├── main.go              # Application entry point
├── gui/                 # GUI package with modular components
│   ├── app.go          # Main window builder
│   ├── home.go         # Home tab component
│   ├── about.go        # About tab component
│   └── settings.go     # Settings tab component
├── go.mod              # Go module definition
├── go.sum              # Go dependencies checksums
├── hello               # Compiled executable (generated)
└── README.md           # This file
```

## Architecture

The project follows idiomatic Go patterns by separating concerns into a dedicated `gui` package:

- **main.go**: Minimal entry point that creates the app and initializes the UI
- **gui/app.go**: Orchestrates the main window and assembles all tabs
- **gui/home.go**: Home tab with welcome content
- **gui/about.go**: About tab with application information
- **gui/settings.go**: Settings tab with preference controls

Each tab is a self-contained function that returns a `*container.TabItem`, making it easy to:
- Modify individual tabs without affecting others
- Add new tabs by creating a new file and function
- Test UI components in isolation

## Requirements

- Go 1.25.5 or later
- Fyne v2.7.0

## Installation

1. Navigate to the project directory:
   ```bash
   cd llm
   ```

2. Download dependencies:
   ```bash
   go mod tidy
   ```

## Building

To build the application, run:

```bash
go build -o hello
```

This will create an executable named `hello` in the current directory.

## Running

Execute the application:

```bash
./hello
```

Or run directly without building:

```bash
go run .
```

## Features

The application demonstrates:

- Modular GUI architecture using separate packages
- Using Fyne's AppTabs widget for tabbed navigation
- Following idiomatic Go conventions for UI building
- Easy-to-extend tab structure
- Clean separation of concerns

## Code Overview

### main.go
Simple entry point that creates the app and window:

```go
package main

import (
	"llm/gui"
	"fyne.io/fyne/v2/app"
)

func main() {
	myApp := app.New()
	myWindow := gui.NewMainWindow(myApp)
	myWindow.ShowAndRun()
}
```

### gui/app.go
Orchestrates the main window and assembles tabs:

```go
func NewMainWindow(app fyne.App) fyne.Window {
	myWindow := app.NewWindow("Hello World with Fyne")
	myWindow.Resize(fyne.NewSize(600, 400))

	tabs := container.NewAppTabs(
		HomeTab(),
		AboutTab(),
		SettingsTab(),
	)

	myWindow.SetContent(tabs)
	return myWindow
}
```

### gui/home.go, gui/about.go, gui/settings.go
Each tab is a standalone function that returns a `*container.TabItem`:

```go
func HomeTab() *container.TabItem {
	content := container.NewVBox(
		widget.NewLabel("Welcome to Fyne!"),
		widget.NewSeparator(),
		// ... more widgets ...
	)
	return container.NewTabItem("Home", content)
}
```

## Adding a New Tab

To add a new tab, follow these steps:

1. Create a new file in the `gui` package (e.g., `gui/newfeature.go`)
2. Implement a function that returns `*container.TabItem`:

   ```go
   package gui

   import (
       "fyne.io/fyne/v2/container"
       "fyne.io/fyne/v2/widget"
   )

   func NewFeatureTab() *container.TabItem {
       content := container.NewVBox(
           widget.NewLabel("New Feature"),
           // Add your widgets here
       )
       return container.NewTabItem("New Feature", content)
   }
   ```

3. Update `gui/app.go` to include the new tab:

   ```go
   tabs := container.NewAppTabs(
       HomeTab(),
       AboutTab(),
       SettingsTab(),
       NewFeatureTab(),  // Add your new tab
   )
   ```

## Dependencies

- `fyne.io/fyne/v2 v2.7.0` - The Fyne GUI framework and all required dependencies

## License

This is a simple example project. Feel free to use and modify it as needed.

## Additional Resources

- [Fyne Documentation](https://docs.fyne.io/)
- [Fyne GitHub Repository](https://github.com/fyne-io/fyne)
- [Fyne Examples](https://docs.fyne.io/explore/hello)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
