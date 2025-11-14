**Tiny Terminal Browser (Go)**

*A minimal exploration of HTML parsing, text extraction, pagination, and terminal rendering.*

📌 Overview

This project is a small experiment in understanding how browsers work at the lowest level—specifically **HTML parsing**, **DOM traversal**, and **rendering text in a terminal**.
It loads a URL, extracts readable text, and displays it in **10-row pages** with simple **Back** and **Forward** navigation.

The goal wasn’t to create a full browser, but to explore the mechanics behind content rendering in a stripped-down environment.

Key Technical Ideas

1. HTML Parsing with `golang.org/x/net/html`**

* Tokenizes and parses HTML into a DOM-like structure.
* Recursive traversal extracts meaningful text while ignoring script/style tags.
* Handles nested elements while maintaining readable formatting in plain text.

2. Pagination Model (10-row chunks)**

* Terminal output is broken into consistent page slices.
* Simple index-based paging allows moving forward/backward without rerendering the whole document.
* Designed to behave like very lightweight virtual scrolling.

3. Browser-like History Stack**

* Navigation history stored in two stacks: *back* and *forward*.
* Mimics the Back/Forward behavior of real browsers, but simplified.
* Supports quick switching between previously loaded pages.

4. Terminal Rendering**

* Clears and redraws the screen on each paging operation.
* Renders text as a unified block of content, independent of HTML layout.
* Keeps the interface minimal and distraction-free.

---

Features

* Load and parse HTML from URLs
* Clean plain-text extraction
* Pagination: **10 lines per page**
* Navigation:

  * **F** — Forward
  * **B** — Back
  * **Q** — Quit
* Simple, dependency-light codebase

---


## 📦 Installation

```bash
git clone https://github.com/yasniy97/main.go
go run main.go
```

---

## 📁 Project Structure

```
.
├── main.go
└── README.md
```

---

## 📝 Notes & Limitations

* Only extracts raw text; ignores layout, styling, images, and complex HTML structures.
* No CSS or JS support (intentionally).
* Basic URL handling and error recovery.
* Designed for learning, not production use.

---

## 🧭 Future Explorations

* Basic hyperlink detection?
* Improving handling of nested lists/tables
* Streamed parsing (for very large documents)
* Optional colorized output

---

## 🛠️ Why I Built This

To better understand what happens between HTML arriving from the network and content appearing on screen.
This project helped me dive into:

* DOM tree structures
* Text extraction heuristics
* Terminal rendering
* Designing simple navigation flows

---

## 🔗 License

MIT


Just tell me!
