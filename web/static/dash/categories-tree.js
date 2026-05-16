document.addEventListener('DOMContentLoaded', function() {
  var selects = document.querySelectorAll('form.category-parent-form select[name="categories"]');
  selects.forEach(function(select) {
    select.addEventListener('change', function() {
      var form = select.closest('form');
      if (form) {
        form.submit();
      }
    });
  });
});
