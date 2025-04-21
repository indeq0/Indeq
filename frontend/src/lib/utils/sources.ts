import type { ChatMessage } from "$lib/types/chat";

export function handleScroll(messages: ChatMessage[], e: Event, messageIndex: number): ChatMessage[] {
    const container = e.target as HTMLElement;
    const atEnd = container.scrollLeft + container.clientWidth >= container.scrollWidth - 10;
    const isScrollable = container.scrollWidth > container.clientWidth;
    
    // Hide tooltips - more thoroughly by targeting both aria attribute and style properties
    const tooltips = document.querySelectorAll('.tooltip');
    tooltips.forEach(tooltip => {
        tooltip.setAttribute('aria-hidden', 'true');
        (tooltip as HTMLElement).style.opacity = '0';
        (tooltip as HTMLElement).style.visibility = 'hidden';
    });
    
    return messages.map((msg, idx) => 
        idx === messageIndex 
            ? {...msg, sourcesScrollAtEnd: atEnd, isScrollable}
            : msg
    );
}

export function checkScrollable(messages: ChatMessage[], container: HTMLElement, messageIndex: number): ChatMessage[] {
    const isScrollable = container.scrollWidth > container.clientWidth;
    return messages.map((msg, idx) => 
        idx === messageIndex 
            ? {...msg, isScrollable}
            : msg
    );
}

export function scrollToPosition(messages: ChatMessage[], element: HTMLElement, messageIndex: number, setMessages: (msgs: ChatMessage[]) => void): void {
    const message = messages[messageIndex];
    if (message.sourcesScrollAtEnd) {
        element.scrollTo({ left: 0, behavior: 'smooth' });
        // Wait for scroll animation to complete before updating state
        element.addEventListener('scrollend', () => {
            setMessages(messages.map((msg, idx) => 
                idx === messageIndex 
                    ? {...msg, sourcesScrollAtEnd: false}
                    : msg
            ));
        }, { once: true });
    } else {
        element.scrollTo({ left: element.scrollWidth, behavior: 'smooth' }); // Add smooth scrolling
        // Wait for scroll animation to complete before updating state
        element.addEventListener('scrollend', () => {
            setMessages(messages.map((msg, idx) => 
                idx === messageIndex 
                    ? {...msg, sourcesScrollAtEnd: true}
                    : msg
            ));
        }, { once: true });
    }
}

// Add the action to check scrollability on mount
export function initScrollCheck(node: HTMLElement, params: { 
    node: HTMLElement | null;
    messages: ChatMessage[]; 
    messageIndex: number;
    setMessages: (msgs: ChatMessage[]) => void;
}) {
    const { messages, messageIndex, setMessages } = params;
    
    setMessages(checkScrollable(messages, node, messageIndex));
    
    const resizeObserver = new ResizeObserver(() => {
        setMessages(checkScrollable(messages, node, messageIndex));
    });
    
    resizeObserver.observe(node);
    
    return {
        destroy() {
            resizeObserver.disconnect();
        }
    };
}

// Function to position tooltips
export function positionTooltip(event: MouseEvent | FocusEvent) {
    const container = event.currentTarget as HTMLElement;
    const tooltipId = container.getAttribute('data-tooltip-id');
    if (!tooltipId) return;
    const tooltip = document.getElementById(tooltipId);
    if (!tooltip) return;
    
    const rect = container.getBoundingClientRect();
    const viewportHeight = window.innerHeight;
    
    // Check if tooltip would go off the bottom of the viewport
    const tooltipHeight = 80; // Approximate height - you may want to calculate this dynamically
    const spaceBelow = viewportHeight - rect.bottom;
    
    if (spaceBelow < tooltipHeight + 10) {
        // If not enough space below, show tooltip above
        tooltip.style.left = `${rect.left}px`;
        tooltip.style.top = `${rect.top - tooltipHeight - 8}px`;
        tooltip.style.transformOrigin = 'bottom center';
    } else {
        // Show tooltip below as usual
        tooltip.style.left = `${rect.left}px`;
        tooltip.style.top = `${rect.bottom + 8}px`;
        tooltip.style.transformOrigin = 'top center';
    }
    
    tooltip.setAttribute('aria-hidden', 'false');
    (tooltip as HTMLElement).style.opacity = '1';
    (tooltip as HTMLElement).style.visibility = 'visible';
}

// Function to hide tooltip
export function hideTooltip(event: MouseEvent | FocusEvent) {
    const container = event.currentTarget as HTMLElement;
    const tooltipId = container.getAttribute('data-tooltip-id');
    if (!tooltipId) return;
    
    const tooltip = document.getElementById(tooltipId);
    if (!tooltip) return;
    tooltip.setAttribute('aria-hidden', 'true');
    (tooltip as HTMLElement).style.opacity = '0';
    (tooltip as HTMLElement).style.visibility = 'hidden';
}