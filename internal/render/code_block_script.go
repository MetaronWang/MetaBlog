package render

func codeBlockScript() string {
	return `<script>
(function() {
  document.querySelectorAll('.code-block').forEach(function(block) {
    var wrapBtn = block.querySelector('.code-block-wrap-btn');
    var copyBtn = block.querySelector('.code-block-copy-btn');
    var collapseBtn = block.querySelector('.code-block-collapse-btn');

    if (wrapBtn) {
      wrapBtn.addEventListener('click', function() {
        var isWrap = block.getAttribute('data-wrap') === 'true';
        block.setAttribute('data-wrap', isWrap ? 'false' : 'true');
      });
    }

    if (copyBtn) {
      copyBtn.addEventListener('click', function() {
        var source = block.querySelector('.code-block-source');
        var text = source ? source.value : '';
        if (!source) {
          var cells = block.querySelectorAll('.code-block-line-code');
          for (var i = 0; i < cells.length; i++) {
            if (i > 0) text += '\n';
            text += cells[i].textContent;
          }
        }
        navigator.clipboard.writeText(text).then(function() {
          copyBtn.style.color = '#a6e22e';
          setTimeout(function() { copyBtn.style.color = ''; }, 1500);
        }).catch(function() {});
      });
    }

    if (collapseBtn) {
      collapseBtn.addEventListener('click', function() {
        block.classList.toggle('code-block-collapsed');
      });
    }
  });
})();
</script>
`
}
