(function(){
  var form = document.getElementById('posts-search-form');
  var input = document.getElementById('posts-search-input');
  if (!form || !input) return;

  function focusSearchToEnd() {
    if (!input.value) {
      return;
    }
    window.requestAnimationFrame(function() {
      input.focus({ preventScroll: true });
      var end = input.value.length;
      input.setSelectionRange(end, end);
    });
  }

  focusSearchToEnd();

  input.addEventListener('search', function() {
    form.requestSubmit();
  });

  input.addEventListener('input', function(event){
    if (event instanceof InputEvent) {
      return;
    }
    form.requestSubmit();
  });
})();
