agent "root" {
  description = "Agent that loads its system prompt from a separate file"
  model       = "auto"

  # The file() helper reads a text file and injects its contents here.
  # Relative paths are resolved from this HCL file's directory.
  # Passing an object as a second argument renders the file as a template:
  # each key becomes a ${key} variable inside the file.
  instruction = file("instructions_from_file.md", {
    audience = "developers"
    style    = "concise"
  })

  welcome_message = "Hi! My instructions were loaded from instructions_from_file.md"
}
