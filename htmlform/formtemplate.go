package htmlform

// FormTemplate is the default HTML template used to render forms.
// It supports multiple input types including text, number, checkbox, select, textarea, and file.
//
// The template expects a struct with:
//   - Title: Page title and heading
//   - Fields: Slice of formField (Name, Label, Type, Value, Required, Options)
//   - SubmitButtonText: Text for the submit button
//
// You can customize this template by parsing your own HTML template
// and assigning it to handler.template after creating the handler.
//
// The template handles these field types specially:
//   - checkbox: Inline label, checked state support
//   - select: Dropdown with options, selected value support
//   - textarea: Multi-line text with 40 cols × 5 rows
//   - all others: Standard <input> tags with type attribute
//
// Example customization:
//
//	handler, _ := htmlform.NewHandler(wrapper, "My Form", resultWriter)
//	handler.template, _ = template.New("form").Parse(myCustomTemplate)
var FormTemplate = /*html*/ `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8"/>
	<meta name="viewport" content="width=device-width,initial-scale=1.0">
	<title>{{.Title}}</title>
	<style>
		:root {
			--bg: #ffffff;
			--text: #222222;
			--link: #0a7ea4;
			--link-visited: #6a4fb3;
			--space: 1rem;
		}
		*, *::before, *::after { box-sizing: border-box; }
		html { 
			margin: 8px;
			padding: 0;
			color-scheme: light;
		}
		body {
			margin: 0;
			padding: 0;
			background: var(--bg);
			color: var(--text);
			font-family: system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif;
			line-height: 1.6;
			font-size: 16px;
			-webkit-font-smoothing: antialiased;
			text-rendering: optimizeLegibility;
		}
		label { display: block; }
		form { margin: 10px; }
		form div { padding-bottom: 10px; }
		p, ul, ol, blockquote, pre { margin: 0 0 var(--space); }
		ul, ol { padding-left: 1.25rem; }
		h1, h2, h3, h4, h5, h6 {
			line-height: 1.2;
			margin: 1.5rem 0 0.5rem;
		}
		a {
			color: var(--link);
			text-underline-offset: 0.15em;
		}
		a:visited { color: var(--link-visited); }
		a:hover, a:focus { text-decoration: underline; }
		:focus-visible {
			outline: 3px solid #ffbf47; /* high-contrast focus */
			outline-offset: 2px;
		}
	</style>
</head>
<body>
<h1>{{.Title}}</h1>
<form method="post" enctype="multipart/form-data">
	{{range .Fields}}
		<div>
			{{if eq .Type "checkbox"}}
				<input type="checkbox" id="{{.Name}}" name="{{.Name}}" value="true" {{if eq .Value "true"}}checked{{end}}/>
				<label style="display: inline" for="{{.Name}}">{{.Label}}</label>
			{{else if eq .Type "select"}}
				<label for="{{.Name}}">{{.Label}}:</label>
				<select id="{{.Name}}" name="{{.Name}}" {{if .Required}}required{{end}}>
					{{$selectValue := .Value}}
					{{range .Options}}
						<option value="{{.Value}}" {{if eq (printf "%v" .Value) $selectValue}}selected{{end}}>{{.Label}}</option>
					{{end}}
				</select>
			{{else if eq .Type "textarea"}}
				<label for="{{.Name}}">{{.Label}}:</label>
				<textarea id="{{.Name}}" name="{{.Name}}" cols="40" rows="5" {{if .Required}}required{{end}}>{{.Value}}</textarea>
			{{else}}
				<label for="{{.Name}}">{{.Label}}:</label>
				<input type="{{.Type}}" id="{{.Name}}" name="{{.Name}}" value="{{.Value}}" size="40" {{if .Required}}required{{end}}/>
			{{end}}
		</div>
	{{end}}
	<button>{{.SubmitButtonText}}</button>
</form>
`
