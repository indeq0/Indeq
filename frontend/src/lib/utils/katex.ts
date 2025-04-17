import katex from 'katex';
import { marked } from 'marked';

// Function to render LaTeX blocks
export function renderLatex(content: string) {
  const tempDiv = document.createElement('div');
  tempDiv.innerHTML = content;

  // Find all LaTeX blocks and render them
  const mathElements = tempDiv.getElementsByClassName('math');
  Array.from(mathElements).forEach((element) => {
    const latex = element.textContent || '';
    try {
      const isDisplay = element.classList.contains('math-display');
      const rendered = katex.renderToString(latex, {
        displayMode: isDisplay,
        throwOnError: false,
        strict: false
      });
      element.innerHTML = rendered;
    } catch (error) {
      console.error('LaTeX rendering error:', error);
    }
  });

  return tempDiv.innerHTML;
}

// Function to render Markdown with LaTeX support
export function renderMarkdown(content: string) {
  // Handle display math: \[ ... \] before markdown processing
  content = content.replace(/\\\[([\s\S]*?)\\\]/g, (match, latex) => {
    return `<div class="math math-display">${latex}</div>`;
  });

  // Handle inline math: \( ... \) before markdown processing
  content = content.replace(/\\\(([\s\S]*?)\\\)/g, (match, latex) => {
    return `<span class="math math-inline">${latex}</span>`;
  });

  return marked(content);
}

// Function to render content with Markdown and LaTeX support
export function renderContent(text: string) {
  // Ensure text is a string and not undefined/null
  if (!text) return '';
  
  // First process markdown
  const markdownRendered = renderMarkdown(text) as string;
  
  // Then process LaTeX
  return renderLatex(markdownRendered);
}
