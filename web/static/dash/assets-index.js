document.addEventListener('DOMContentLoaded', function() {
  var assetAPIBase = "";
  var uploadRoot = document.getElementById('asset-upload-root');
  var fileInput = document.getElementById('asset-drop-input');
  var uploadRemark = document.getElementById('asset-upload-remark');
  var alertDialog = document.getElementById('asset-alert-dialog');
  var alertMessage = document.getElementById('asset-alert-message');
  var isUploading = false;
  var uploadReady;
  var uploadError;

  if (!uploadRoot || !fileInput || !uploadRemark) {
    return;
  }

  assetAPIBase = (uploadRoot.getAttribute('data-asset-api-base') || '').trim();
  uploadReady = uploadRoot.getAttribute('data-upload-ready') === '1';
  uploadError = uploadRoot.getAttribute('data-upload-error') || '';

  function removeAssetRows(ids) {
    ids.forEach(function(id) {
      document.querySelectorAll('tr[data-asset-id="' + id + '"]').forEach(function(row) {
        row.remove();
      });
    });
  }

  function refreshMultiselectState() {
    document.dispatchEvent(new CustomEvent('dash:multiselect:refresh', {
      detail: {
        tableId: 'assets-table'
      }
    }));
  }

  function showAssetNotice(message, title) {
    var heading = title || '提示';
    var titleNode = alertDialog ? alertDialog.querySelector('.ui-dialog-title') : null;

    if (titleNode) {
      titleNode.textContent = heading;
    }
    if (alertMessage) {
      alertMessage.textContent = message;
    }
    if (!window.DashAppUI.dialog.open('asset-alert-dialog')) {
      notify(message, heading, {
        variant: 'warning',
      });
    }
  }

  function showAssetToast(message, title, variant) {
    notify(message, title || '提示', {
      variant: variant || 'info'
    });
  }

  function askConfirm(message, title) {
    return window.DashAppUI.confirm.ask({
        dialogId: 'asset-confirm-dialog',
        title: title,
        message: message,
        messageSelector: '#asset-confirm-message',
        okSelector: '#asset-confirm-ok',
      });
  }

  function setUploading(working) {
    isUploading = working;
    uploadRoot.classList.toggle('is-uploading', working);
  }

  function uploadSingleFile(file) {
    var fd = new FormData();
    var remark = (uploadRemark.value || '').trim();

    fd.append('file', file);
    if (remark) {
      fd.append('remark', remark);
    }

    return sfetchJSON(assetAPIBase, {
      method: 'POST',
      body: fd
    }).then(function(ret) {
      if (!ret.body || ret.body.ok !== true) {
        var msg = (ret.body && ret.body.error) || ret.raw || ('上传失败，状态码 ' + ret.status);
        throw new Error(msg);
      }
      return ret.body;
    });
  }

  function compactErrorMessage(message) {
    var text = (message || '').replace(/\s+/g, ' ').trim();
    if (!text) {
      return '上传失败';
    }
    if (text.length > 320) {
      return text.slice(0, 320) + '...';
    }
    return text;
  }

  function handleFiles(fileList) {
    var files;
    var confirmText;

    if (!uploadReady) {
      showAssetNotice(uploadError || '上传服务配置不完整，请先到设置里完成配置');
      return;
    }

    if (isUploading) {
      return;
    }

    files = Array.from(fileList || []);
    if (!files.length) {
      return;
    }

    confirmText = files.length === 1
      ? ('确认上传文件 "' + files[0].name + '" ?')
      : ('确认上传这 ' + files.length + ' 个文件？');

    askConfirm(confirmText, '确认上传').then(function(confirmed) {
      var successCount = 0;
      var failedCount = 0;
      var failedNames = [];
      var failedDetails = [];
      var chain = Promise.resolve();

      if (!confirmed) {
        return;
      }

      setUploading(true);

      files.forEach(function(file) {
        chain = chain.then(function() {
          return uploadSingleFile(file).then(function() {
            successCount += 1;
          }).catch(function(err) {
            var name = file.name || '（未命名）';

            failedCount += 1;
            failedNames.push(name);
            failedDetails.push(name + '：' + compactErrorMessage(err && err.message));
          });
        });
      });

      chain.finally(function() {
        if (failedCount > 0) {
          var message = '上传完成：成功 ' + successCount + '，失败 ' + failedCount + '。失败文件：' + failedNames.join('、');
          if (failedDetails.length > 0) {
            var detailText = failedDetails.slice(0, 3).join('；');
            if (failedDetails.length > 3) {
              detailText += '；...';
            }
            message += '。失败原因：' + detailText;
          }
          showAssetNotice(message);
        } else {
          showAssetToast('上传完成：共 ' + successCount + ' 个文件', '资源上传', 'success');
        }

        if (successCount > 0 && failedCount === 0) {
          window.setTimeout(function() {
            goTo('', { reload: true });
          }, 900);
          return;
        }

        if (successCount > 0 && failedCount > 0) {
          setUploading(false);
          return;
        }

        setUploading(false);
      });
    });
  }

  function copyToClipboard(text) {
    if (!text) {
      return Promise.reject(new Error('empty text'));
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }

    return new Promise(function(resolve, reject) {
      var temp = document.createElement('textarea');
      var ok;

      temp.value = text;
      temp.style.position = 'fixed';
      temp.style.top = '-999px';
      temp.style.left = '-999px';
      document.body.appendChild(temp);
      temp.focus();
      temp.select();

      try {
        ok = document.execCommand('copy');
        temp.remove();
        if (!ok) {
          reject(new Error('copy failed'));
          return;
        }
        resolve();
      } catch (err) {
        temp.remove();
        reject(err);
      }
    });
  }

  function markdownImage(url, name) {
    var alt = (name || '').trim();

    alt = alt.replace(/\[/g, '\\[').replace(/\]/g, '\\]');
    return '![' + alt + '](' + url + ')';
  }

  fileInput.addEventListener('change', function() {
    handleFiles(fileInput.files);
    fileInput.value = '';
  });

  document.addEventListener('click', function(event) {
    var copyURLButton = event.target.closest('.asset-copy-url');
    var copyMarkdownButton = event.target.closest('.asset-copy-markdown');
    var deleteButton = event.target.closest('.asset-delete');
    var id;

    if (copyURLButton) {
      event.preventDefault();
      copyToClipboard(copyURLButton.getAttribute('data-url') || '').then(function() {
        showAssetNotice('已复制链接');
      }).catch(function(err) {
        console.warn('copy url failed', err);
        showAssetNotice('复制失败');
      });
      return;
    }

    if (copyMarkdownButton) {
      event.preventDefault();
      copyToClipboard(markdownImage(
        copyMarkdownButton.getAttribute('data-url') || '',
        copyMarkdownButton.getAttribute('data-name') || ''
      )).then(function() {
        showAssetNotice('已复制 Markdown');
      }).catch(function(err) {
        console.warn('copy markdown failed', err);
        showAssetNotice('复制失败');
      });
      return;
    }

    if (!deleteButton) {
      return;
    }

    event.preventDefault();
    id = Number(deleteButton.getAttribute('data-id') || '0');
    if (!id) {
      showAssetNotice('无效 ID');
      return;
    }

    askConfirm('确认删除该资源？会同时删除远端文件。', '确认删除').then(function(confirmed) {
      if (!confirmed) {
        return;
      }

      sfetchJSON(assetAPIBase + '/' + id, {
        method: 'DELETE',
      }).then(function(ret) {
        if (!ret.body || ret.body.ok !== true) {
          var msg = (ret.body && ret.body.error) || ret.raw || ('删除失败，状态码 ' + ret.status);
          throw new Error(msg);
        }
        removeAssetRows([id]);
        refreshMultiselectState();
      }).catch(function(err) {
        showAssetNotice(err.message || '删除失败');
      });
    });
  });

  uploadRemark.addEventListener('click', function(event) {
    event.stopPropagation();
  });
});
