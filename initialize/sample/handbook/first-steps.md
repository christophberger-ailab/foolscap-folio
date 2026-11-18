---
title: First steps
order: 1
---

# First steps

This page is a Markdown file directly in the topic folder, so it is a **section** of the book. In print, its first heading becomes the section's title and the rest of the page the section's introduction; `_brand.yml` sets their fonts under `meta: fonts: section:`.

Start every page with one heading of level 1 (`#`), and use levels 2 and below (`##`, `###`, …) for the rest of the page. Folio shifts the levels for print to match where a page sits in the tree, so you never have to renumber headings when you move a page.

## Configure the project

`folio.yml` in the project root lists the topics and the output formats. The website format `web` is set up already; the file's comments show how to add a printed handout.

## Brand the output

`_brand.yml` in the project root sets the colors, fonts and logo of every output. Change `color: primary:` to your own color and render again to see it in the navbar.

## Render

Run `folio render` to write every format to the `_output` folder.
