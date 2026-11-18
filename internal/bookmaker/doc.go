// Package bookmaker flattens a topic folder into a single Markdown
// document that folio renders as a DOCX or PDF, plus a second document
// holding the slides those pages carry.
//
// The site expresses structure through nested folders, while a document is
// one text. The bookmaker concatenates a folder's pages into one text in
// website order. The topic's index is the book's preliminaries, the level
// below it holds sections and the next level chapters, which all keep
// their headings as written; the headings of deeper pages are demoted by
// one level for each level further down (issue #96).
//
// The package is a copy of the quarto-bookmaker command's internal package
// (https://github.com/christophberger-ailab/quarto-bookmaker, MIT), which
// cannot be imported directly because Go's internal rule keeps it inside its
// own module. It reached folio by way of quarto-sorter, which carried the same
// copy. Keep changes in sync with upstream.
package bookmaker
