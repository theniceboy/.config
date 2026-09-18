---
name: imagegen
description: Generate images (photos, illustrations, icons, mockups, any visual asset) using GPT Image via ChatGPT subscription. Use whenever the user asks to create, generate, draw, or produce an image or visual.
user-invokable: true
args:
  - name: prompt
    description: The image generation prompt describing what to create
    required: true
---

# Image Generation

You have a custom tool called `imagegen` that generates images using GPT Image (gpt-image-2) via the user's ChatGPT subscription. **Always prefer this tool** over Python/Pillow/svglib/HTML-canvas or any other approach for creating images.

## When to use

- User asks to create, generate, draw, make, or produce any image, photo, illustration, mockup, icon, visual asset, or picture
- User asks for concept art, product shots, UI mockups, logos, sprites, textures, or any raster image
- User says "show me what X looks like" or "make a picture of X"

## How to use

Call the `imagegen` tool with a single `prompt` argument. It returns the file path of the saved PNG.

```
tool: imagegen
args: { prompt: "<detailed description of the image>" }
```

## Prompt tips

- Be specific about style (photo, watercolor, 3D render, flat illustration, etc.)
- Describe composition, lighting, colors, and mood
- For text in images, quote exact text and spell out tricky words letter by letter
- Specify the use case (product photo, game asset, website hero, etc.)

## After generation

The tool returns a file path like `/tmp/codex-imagen-...png`. You can then:
- Tell the user the file path
- Use `open` (macOS) to display it: `open /tmp/image.png`
- Move it into the project workspace if needed

## Do NOT

- Do not use Python, Pillow, matplotlib, or any code to generate images when the imagegen tool is available
- Do not create SVG or HTML placeholders when the user explicitly asked for an image
- Do not attempt to use DALL-E or any other image API directly
