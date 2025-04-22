import katex from 'katex';
import { marked } from 'marked';
import Prism from 'prismjs';
import { toast } from 'svelte-sonner';

// Import Prism language components - add more as needed
import 'prismjs/components/prism-javascript';
import 'prismjs/components/prism-typescript';
import 'prismjs/components/prism-css';
import 'prismjs/components/prism-json';
import 'prismjs/components/prism-python';
import 'prismjs/components/prism-bash';
import 'prismjs/components/prism-jsx';
import 'prismjs/components/prism-tsx';
import 'prismjs/components/prism-markdown';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-sql';
import 'prismjs/components/prism-java';
import 'prismjs/components/prism-c';
import 'prismjs/components/prism-cpp';

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

// Function to get highlighted code with Prism
function highlightCode(code: string, language: string): string {
  const normalizedLang = language.toLowerCase();
  
  try {
    if (Prism.languages[normalizedLang]) {
      return Prism.highlight(code, Prism.languages[normalizedLang], normalizedLang);
    }
    
    const languageMap: Record<string, string> = {
      'js': 'javascript',
      'ts': 'typescript',
      'py': 'python',
      'sh': 'bash',
      'shell': 'bash',
      'zsh': 'bash',
      'yml': 'yaml',
      'html': 'markup',
      'xml': 'markup',
      'svg': 'markup',
      'mathml': 'markup',
      'react': 'jsx',
      'svelte': 'markup'
    };
    
    const mappedLang = languageMap[normalizedLang];
    if (mappedLang && Prism.languages[mappedLang]) {
      return Prism.highlight(code, Prism.languages[mappedLang], mappedLang);
    }
    
    // If no highlighting is available, escape HTML
    return code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  } catch (error) {
    console.error('Syntax highlighting error:', error);
    // Return escaped HTML on error
    return code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
}

// Process inline code blocks surrounded by single backticks
function processInlineCodeBlocks(html: string): string {
  // This regex finds text surrounded by single backticks (but not triple backticks used for code blocks)
  // and not already inside a code element or pre element
  const regex = /(?<!`{2})(`)((?!\1).+?)(\1)(?!`{2})(?![^<]*<\/code>|[^<]*<\/pre>)/g;
  
  // Remove the backticks and wrap the content in a code tag
  return html.replace(regex, (match, openingTick, content, closingTick) => {
    // Escape HTML in the content to prevent injection
    const escapedContent = content
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
      
    return `<code class="inline-code" style="font-family: 'Work Sans', monospace; border-radius: 6px;">${escapedContent}</code>`;
  });
}

function enhanceCodeBlocks(html: string): string {
  // First process any inline code blocks that might not be caught by marked
  html = processInlineCodeBlocks(html);
  
  const tempDiv = document.createElement('div');
  tempDiv.innerHTML = html;
  
  const codeBlocks = tempDiv.querySelectorAll('pre > code');
  
  codeBlocks.forEach(codeElement => {
    const preElement = codeElement.parentElement;
    if (!preElement) return;
    
    // Get language from class (language-xxx)
    let language = 'text';
    for (const className of codeElement.classList) {
      if (className.startsWith('language-')) {
        language = className.replace('language-', '');
        break;
      }
    }
    
    // Format language name to be more readable
    const formattedLanguage = formatLanguageName(language);
    
    const code = codeElement.textContent || '';
    
    const wrapper = document.createElement('div');
    wrapper.className = 'code-block-wrapper';
    
    const header = document.createElement('div');
    header.className = 'code-block-header';
    
    const langSpan = document.createElement('span');
    langSpan.className = 'code-language';
    langSpan.textContent = formattedLanguage;
    
    const copyButton = document.createElement('button');
    copyButton.className = 'copy-code-button';
    copyButton.setAttribute('data-code', encodeURIComponent(code));
    copyButton.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>';
    
    header.appendChild(langSpan);
    header.appendChild(copyButton);
    
    const newPre = document.createElement('pre');
    const newCode = document.createElement('code');
    newCode.className = codeElement.className;
    
    if (language !== 'text') {
      newCode.innerHTML = highlightCode(code, language);
    } else {
      // plain text styling
      newCode.innerHTML = `<span style="color: black;">${code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</span>`;
    }
    
    newPre.appendChild(newCode);
    
    wrapper.appendChild(header);
    wrapper.appendChild(newPre);
    
    const parent = preElement.parentElement;
    if (parent) {
      parent.replaceChild(wrapper, preElement);
    }
  });
  
  // Process all inline code elements
  const inlineCodeElements = tempDiv.querySelectorAll('code:not(pre > code)');
  inlineCodeElements.forEach(element => {
    // Add the inline-code class if not already present
    if (!element.classList.contains('inline-code')) {
      element.classList.add('inline-code');
    }
    
    // Set Work Sans font if element is an HTMLElement
    if (element instanceof HTMLElement) {
      element.style.fontFamily = "'Work Sans', monospace";
      element.style.borderRadius = "6px"; // Make inline code more rounded
    }
    
    // Remove backticks if they exist
    let content = element.textContent || '';
    if (content.startsWith('`') && content.endsWith('`')) {
      content = content.substring(1, content.length - 1);
    }
    
    // Remove any remaining backticks (sometimes they can be embedded differently)
    content = content.replace(/`/g, '');
    element.textContent = content;
  });
  
  return tempDiv.innerHTML;
}

// Function to format language names to be more readable
function formatLanguageName(language: string): string {
  // Map of language codes to friendly names
  const languageMap: Record<string, string> = {
    'js': 'JavaScript',
    'javascript': 'JavaScript',
    'ts': 'TypeScript',
    'typescript': 'TypeScript',
    'jsx': 'JSX',
    'tsx': 'TSX',
    'css': 'CSS',
    'html': 'HTML',
    'xml': 'XML',
    'json': 'JSON',
    'py': 'Python',
    'python': 'Python',
    'rb': 'Ruby',
    'ruby': 'Ruby',
    'java': 'Java',
    'c': 'C',
    'cpp': 'C++',
    'cs': 'C#',
    'csharp': 'C#',
    'go': 'Go',
    'rust': 'Rust',
    'php': 'PHP',
    'swift': 'Swift',
    'kotlin': 'Kotlin',
    'scala': 'Scala',
    'sql': 'SQL',
    'sh': 'Shell',
    'bash': 'Bash',
    'shell': 'Shell',
    'yaml': 'YAML',
    'yml': 'YAML',
    'markdown': 'Markdown',
    'md': 'Markdown',
    'text': 'Plain Text',
    'plaintext': 'Plain Text',
    'svelte': 'Svelte'
  };
  
  // Return the friendly name if it exists, otherwise capitalize the first letter
  if (languageMap[language.toLowerCase()]) {
    return languageMap[language.toLowerCase()];
  }
  
  // Capitalize first letter and format multi-word languages
  if (language.includes('-')) {
    // Handle hyphenated names like "objective-c"
    return language.split('-')
      .map(part => part.charAt(0).toUpperCase() + part.slice(1))
      .join('-');
  }
  
  return language.charAt(0).toUpperCase() + language.slice(1);
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

  // Process markdown with marked
  const htmlContent = marked.parse(content) as string;
  
  // Enhance code blocks with language and copy buttons
  return enhanceCodeBlocks(htmlContent);
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

// Helper function to initialize copy buttons functionality
export function initCodeCopyButtons() {
  // This function should be called after the DOM has been updated
  setTimeout(() => {
    document.querySelectorAll('.copy-code-button:not([data-initialized])').forEach(button => {
      if (button instanceof HTMLElement) {
        // Mark the button as initialized
        button.setAttribute('data-initialized', 'true');
        
        button.addEventListener('click', () => {
          const code = decodeURIComponent(button.getAttribute('data-code') || '');
          navigator.clipboard.writeText(code);
          
          // Show visual feedback on the button
          const originalHTML = button.innerHTML;
          button.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="copied-icon"><polyline points="20 6 9 17 4 12"></polyline></svg>';
          
          toast.success('Code copied to clipboard', {
            duration: 750
          });
          
          setTimeout(() => {
            button.innerHTML = originalHTML;
          }, 1500);
        });
      }
    });
  }, 0);
}
