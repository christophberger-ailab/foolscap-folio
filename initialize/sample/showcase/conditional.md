---
title: Conditional content
order: 1
---

# Conditional content

`folio.yml` lists the classes a format or an audience shows. A block or span with such a class is kept only where it is shown.

::: explanation
The website shows this block, because its format lists `explanation` under `show:`. The handout leaves it out.
:::

::: experts
Only the audience `experts` reads this block.
:::

Everybody reads this sentence[, and the experts read this part of it too]{.experts}.

- A list item for everyone
[- A list item for the experts]{.experts}
- Another one for everyone

::: {.content-visible when-format="html"}
This block is in the website only.
:::

::: {.content-hidden when-format="html"}
This block is in the DOCX and the PDF only.
:::
