package web

// copyedit_defaults.go — the editing tasks a fresh installation starts
// with. They are written into config.cue on first start like any task the
// user adds, so from then on they are the user's to edit or delete.

// defaultCopyeditPrompts returns the shipped editing tasks. It returns a
// new slice on every call, so no config can change another's tasks.
func defaultCopyeditPrompts() []copyeditPrompt {
	return []copyeditPrompt{{
		ID:    "p1",
		Title: "Orthography: Spelling, Grammar, and Punctuation",
		Prompt: `Check the text for spelling, grammatical, and punctuation errors. Specific rules to obey:

- No Oxford comma.
- Always use straight quotes.
- Lists must be preceded by an empty line.
- The starting fence of a Quarto block (:::) must be preceded by an empty line.
- Quarto block fences can have more then three colons. The number of colons should reflect the level of nesting in other Quarto blocks (More colons -> deeper level).

In any case, only flag issues that need fixing. Don't report things that are already ok.`,
	}, {
		ID:     "p2",
		Title:  "Actions: Turn Nominalizations Back Into Verbs",
		Prompt: `Turn nominalizations (abstract nouns made from verbs, e.g., "implementation" instead of "implement") back into verbs. Sentences are clearer when actions are expressed as verbs, not buried as nouns.`,
	}, {
		ID:     "p3",
		Title:  "Characters: Make the Main Character the Grammatical Subject",
		Prompt: `Make the main characters (people or things the sentence is about) the grammatical subjects. Avoid burying them in prepositional phrases or passive constructions unless there's a good reason (like emphasis or unknown agency).`,
	}, {
		ID:     "p4",
		Title:  "Cohesion: Connect Each Sentence Logically to the Previous One",
		Prompt: `Sentences should flow by starting with familiar/old information and ending with new information, so each sentence connects logically to the previous one.`,
	}, {
		ID:     "p5",
		Title:  "Emphasis: Put Significant Information at the End of a Sentence",
		Prompt: `The end of a sentence is where readers place the most stress/importance, so put the most significant new information there, not buried in the middle.`,
	}, {
		ID:     "p6",
		Title:  "Coherence: Maintain a Consistent Thread Across a Paragraph",
		Prompt: `Beyond individual sentences, whole paragraphs need a consistent thread — usually by keeping the same characters as subjects across sentences and maintaining a clear topic string.`,
	}, {
		ID:     "p7",
		Title:  "Concision: Every Word Should Earn its Place",
		Prompt: `Cut redundant words, meaningless qualifiers, and needless throat-clearing phrases ("it is important to note that..."). Every word should earn its place.`,
	}, {
		ID:     "p8",
		Title:  "Shape: Break Up Complex Sentences",
		Prompt: `Break up long, tangled sentences; avoid overly complex nesting of clauses so readers aren't forced to hold too much in working memory.`,
	}}
}
