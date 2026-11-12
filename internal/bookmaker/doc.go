// Package bookmaker flattens a topic folder into a single Markdown
// document that folio renders as a DOCX or PDF, plus a second document
// holding the slides those pages carry.
//
// The site expresses structure through nested folders, while a document is
// one text. The bookmaker concatenates a folder's pages into one text in
// website order, shifting each page's headings to the level its folder
// depth implies.
//
// The package is a copy of the quarto-bookmaker command's internal package
// (https://github.com/christophberger-ailab/quarto-bookmaker, MIT), which
// cannot be imported directly because Go's internal rule keeps it inside its
// own module. It reached folio by way of quarto-sorter, which carried the same
// copy. Keep changes in sync with upstream.
package bookmaker
