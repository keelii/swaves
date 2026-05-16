document.addEventListener('DOMContentLoaded', function() {
  var trigger = document.getElementById('trash-empty-trigger');
  if (!trigger) {
    return;
  }

  var currentTrashLabel = trigger.getAttribute('data-current-trash-label') || '';
  var batchDeleteURL = (trigger.getAttribute('data-batch-delete-url') || '').trim();
  var confirmAPI = window.DashAppUI.confirm;
  var toastAPI = window.DashAppUI.toast;

  function currentIDs() {
    return Array.from(document.querySelectorAll('#trash-table tbody tr[data-batch-delete-id]')).map(function(row) {
      var id = parseInt(row.getAttribute('data-batch-delete-id') || '0', 10);
      if (!Number.isFinite(id) || id <= 0) {
        return 0;
      }
      return id;
    }).filter(function(id) {
      return id > 0;
    });
  }

  function showToast(message, kind) {
    toastAPI.show({
      kind: kind || 'info',
      title: '回收站',
      message: message,
      duration: 2800,
    });
  }

  trigger.addEventListener('click', function(event) {
    event.preventDefault();

    var ids = currentIDs();
    if (ids.length === 0) {
      trigger.disabled = true;
      return;
    }

    var message = '确定彻底删除当前“' + currentTrashLabel + '”下的 ' + ids.length + ' 项吗？此操作不可恢复。';
    var ask = confirmAPI.ask({
        dialogId: 'trash-empty-confirm-dialog',
        title: '确认全部清空',
        message: message,
        messageSelector: '#trash-empty-confirm-message',
        opener: trigger,
      });

    ask.then(function(confirmed) {
      if (!confirmed) {
        return;
      }

      trigger.disabled = true;
      return window.sfetchJSON(batchDeleteURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ ids: ids }),
      }).then(function(result) {
        var body = result && result.body ? result.body : {};
        var deletedCount = Number(body.deleted_count || 0);
        var failedCount = Number(body.failed_count || 0);

        if (failedCount > 0) {
          showToast('已清空 ' + deletedCount + ' 项，另有 ' + failedCount + ' 项删除失败。', 'warning');
        } else {
          showToast('已清空当前' + currentTrashLabel + '。', 'success');
        }

        window.setTimeout(function() {
          goTo('', { reload: true });
        }, 700);
      }).catch(function() {
        trigger.disabled = false;
        showToast('全部清空失败，请稍后重试。', 'danger');
      });
    });
  });
});
