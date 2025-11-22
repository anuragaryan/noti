// Simple Markdown Parser
export default {
    parse(markdown) {
        if (!markdown) return '';
        
        let html = markdown;
        
        // Escape HTML
        html = this.escapeHtml(html);
        
        // Headers
        html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
        html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
        html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
        
        // Bold
        html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
        html = html.replace(/__(.+?)__/g, '<strong>$1</strong>');
        
        // Italic
        html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
        html = html.replace(/_(.+?)_/g, '<em>$1</em>');
        
        // Code blocks
        html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>');
        
        // Inline code
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
        
        // Links
        html = html.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2">$1</a>');
        
        // Images
        html = html.replace(/!\[([^\]]*)\]\(([^\)]+)\)/g, '<img src="$2" alt="$1">');
        
        // Blockquotes
        html = html.replace(/^\> (.+)/gim, '<blockquote>$1</blockquote>');
        
        // Horizontal rule
        html = html.replace(/^---$/gim, '<hr>');
        html = html.replace(/^\*\*\*$/gim, '<hr>');
        
        // Unordered lists
        html = html.replace(/^\* (.+)/gim, '<li>$1</li>');
        html = html.replace(/^- (.+)/gim, '<li>$1</li>');
        html = html.replace(/(<li>.*<\/li>)/s, '<ul>$1</ul>');
        
        // Ordered lists
        html = html.replace(/^\d+\. (.+)/gim, '<li>$1</li>');
        
        // Line breaks and paragraphs
        html = html.split('\n\n').map(para => {
            if (para.match(/^<(h[1-6]|ul|ol|pre|blockquote|hr)/)) {
                return para;
            }
            return '<p>' + para.replace(/\n/g, '<br>') + '</p>';
        }).join('\n');
        
        return html;
    },
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
};