---
name: classify-checkout
description: Classify an ODH/RHOAI component checkout.
allowed-tools: Read, Glob, Grep
---

# Classify Checkout

Examine the files in [checkout] to classify the project as one of the following:

1. operator
2. library
3. service
4. configuration
5. other (explain what you think it should be)

You will write the result to the destination defined by --ouput in the shape of this json ...

{
  "checkout": "[checkout]",
  "classification": "<classification>",
  "files_read": [<list of filepaths (relative to the checkout path) you read to make the judgement>]
  "reasoning": "<your justification or reasoning for the classification>"
}

## Arguments

- `[checkout]` -- checkout path
- `--output=PATH`, where to write the result.
- `--generated-by=STRING`, optional metadata value.
- `--component-name=STRING`, name of the component.
