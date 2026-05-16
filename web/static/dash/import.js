document.addEventListener('DOMContentLoaded', function() {
  var pageRoot = document.querySelector('[data-import-page]');
  if (!pageRoot) {
    return;
  }
  var parseItemURL = (pageRoot.getAttribute('data-parse-item-url') || '').trim();
  var saveItemURL = (pageRoot.getAttribute('data-save-item-url') || '').trim();
  var confirmItemURL = (pageRoot.getAttribute('data-confirm-item-url') || '').trim();
  var confirmAllURL = (pageRoot.getAttribute('data-confirm-all-url') || '').trim();
  var cancelItemURL = (pageRoot.getAttribute('data-cancel-item-url') || '').trim();
  var baseCategoryOptions = Array.prototype.slice.call(document.querySelectorAll('#import-base-category-options option')).map(function(option) {
    return option.value || '';
  }).filter(Boolean);

  var form = document.getElementById('form');
  var fileInput = document.getElementById('import-files-input');
  var uploadRoot = document.getElementById('import-upload-root');
  var editPanel = document.getElementById('import-edit-panel');
  var editSummary = document.getElementById('import-edit-summary');
  var editSummaryBody;
  var editTableWrap = document.getElementById('import-edit-table-wrap');
  var editEmptyState = document.getElementById('import-edit-empty-state');
  var editRows = document.getElementById('import-edit-rows');
  var confirmBtn = document.getElementById('import-confirm-btn');
  var cancelBtn = document.getElementById('import-cancel-btn');
  var cancelConfirmBtn = document.getElementById('import-cancel-confirm-btn');
  var rowTemplate = document.getElementById('import-edit-row-template');
  var toggleImportStatusBtn = document.getElementById('toggle-import-status-btn');
  var parseConcurrency = 50;
  var activeSaveRequests = 0;
  var importingTotalCount = Number(pageRoot.getAttribute('data-importing-total') || '0') || 0;
  var selectedFiles = [];
  var isParsing = false;
  var isConfirming = false;
  var isCancelling = false;

  if (!form || !fileInput || !uploadRoot || !editPanel || !editSummary || !editTableWrap ||
      !editEmptyState || !editRows || !confirmBtn || !cancelBtn || !cancelConfirmBtn || !rowTemplate ||
      !toggleImportStatusBtn) {
    return;
  }

  editSummaryBody = editSummary.querySelector('.ui-alert-body');

  function qs(selector, root) {
    return (root || document).querySelector(selector);
  }

  function qsa(selector, root) {
    return Array.from((root || document).querySelectorAll(selector));
  }

  function show(target, display) {
    var element = typeof target === 'string' ? qs(target) : target;
    if (element) {
      element.style.display = display || 'block';
    }
  }

  function hide(target) {
    var element = typeof target === 'string' ? qs(target) : target;
    if (element) {
      element.style.display = 'none';
    }
  }

  function setDisabled(elements, disabled) {
    qsa('input, select, button, textarea', elements).forEach(function(element) {
      element.disabled = disabled;
    });
  }

  function readControlValue(selector, fallbackValue) {
    var element = qs(selector, form);
    return (element && element.value) || fallbackValue || '';
  }

  function rowField(rowEl, fieldName) {
    return qs('[data-field="' + fieldName + '"]', rowEl);
  }

  function rowFields(rowEl, fieldName) {
    return qsa('[data-field="' + fieldName + '"]', rowEl);
  }

  function buildImportPageURL(page) {
    var nextPage = Number(page || 1);
    if (!Number.isFinite(nextPage) || nextPage < 1) {
      nextPage = 1;
    }
    var url = new URL(window.location.href);
    url.searchParams.set('page', nextPage);
    return url.pathname + url.search;
  }

  function normalizeConcurrency(value, fallback) {
    var num = Number(value);
    if (!Number.isFinite(num) || num < 1) {
      return fallback;
    }
    return Math.max(1, Math.floor(num));
  }

  async function runWithConcurrency(items, concurrency, worker) {
    var list = Array.isArray(items) ? items : [];
    var workers = [];
    var nextIndex = 0;
    var i;
    var limit;

    if (list.length === 0) {
      return;
    }

    limit = normalizeConcurrency(concurrency, 1);
    if (limit > list.length) {
      limit = list.length;
    }

    async function runWorker() {
      while (true) {
        var index = nextIndex;
        nextIndex += 1;
        if (index >= list.length) {
          return;
        }
        await worker(list[index], index, list.length);
      }
    }

    for (i = 0; i < limit; i++) {
      workers.push(runWorker());
    }
    await Promise.all(workers);
  }

  function normalizeImportListValue(raw) {
    var value = String(raw == null ? '' : raw).trim();
    if (!value) {
      return '';
    }
    value = value.replace(/^["']+|["']+$/g, '').trim();
    return value;
  }

  function splitCSV(raw) {
    var value = raw || '';
    var seen = {};
    var list = [];

    if (!value.trim()) {
      return [];
    }

    value.split(',').forEach(function(part) {
      var item = normalizeImportListValue(part);
      if (!item || seen[item]) {
        return;
      }
      seen[item] = true;
      list.push(item);
    });
    return list;
  }

  function appendUnique(list, value) {
    value = normalizeImportListValue(value);
    if (!value) {
      return list;
    }
    if (list.indexOf(value) >= 0) {
      return list;
    }
    list.push(value);
    return list;
  }

  function selectedFilesFromInput() {
    if (!fileInput.files) {
      return [];
    }
    return Array.from(fileInput.files);
  }

  function setInputFiles(files) {
    if (typeof DataTransfer !== 'function') {
      return;
    }

    try {
      var dt = new DataTransfer();
      (files || []).forEach(function(file) {
        dt.items.add(file);
      });
      fileInput.files = dt.files;
    } catch (err) {
      console.warn('set input files failed', err);
    }
  }

  function setSelectedFiles(files) {
    selectedFiles = Array.from(files || []);
    setInputFiles(selectedFiles);
  }

  function normalizeItem(raw) {
    var postID;
    var status;
    var kind;

    raw = raw || {};
    postID = Number(raw.post_id || raw.PostID || 0);
    status = String(raw.status || raw.Status || 'draft').toLowerCase() === 'published' ? 'published' : 'draft';
    kind = String(raw.kind || raw.Kind || '0') === '1' ? '1' : '0';

    return {
      post_id: postID,
      filename: String(raw.filename || raw.Filename || ''),
      title: String(raw.title || raw.Title || ''),
      slug: String(raw.slug || raw.Slug || ''),
      content_preview: String(raw.content_preview || raw.ContentPreview || ''),
      status: status,
      kind: kind,
      created_at: String(raw.created_at || raw.CreatedAt || ''),
      created_at_unix: Number(raw.created_at_unix || raw.CreatedAtUnix || 0),
      tags: String(raw.tags || raw.Tags || ''),
      category: normalizeImportListValue(raw.category || raw.Category || ''),
      categories: splitCSV(raw.categories || raw.Categories || '').join(', ')
    };
  }

  function createImportRow(item) {
    var tpl = rowTemplate;
    var rowEl;
    var contentNode;
    var categorySelect;
    var statusName;
    var statusValue;
    var kindName;
    var kindValue;

    if (!tpl.content) {
      return null;
    }

    rowEl = tpl.content.firstElementChild ? tpl.content.firstElementChild.cloneNode(true) : null;
    if (!rowEl) {
      return null;
    }

    rowEl.setAttribute('data-post-id', item.post_id);
    rowField(rowEl, 'post_id').value = item.post_id;
    rowField(rowEl, 'filename').value = item.filename;
    rowField(rowEl, 'created_at_unix').value = item.created_at_unix;
    rowField(rowEl, 'title').value = item.title;
    rowField(rowEl, 'slug').value = item.slug;
    rowField(rowEl, 'created_at').value = item.created_at;
    rowField(rowEl, 'tags').value = item.tags;
    rowField(rowEl, 'categories').value = item.categories;

    contentNode = qs('.import-item-content', rowEl);
    if (contentNode) {
      contentNode.textContent = item.content_preview ? item.content_preview : '-';
    }

    statusName = 'import_status_' + item.post_id;
    statusValue = item.status === 'published' ? 'published' : 'draft';
    rowFields(rowEl, 'status').forEach(function(input) {
      input.name = statusName;
      input.checked = input.value === statusValue;
    });

    kindName = 'import_kind_' + item.post_id;
    kindValue = item.kind === '1' ? '1' : '0';
    rowFields(rowEl, 'kind').forEach(function(input) {
      input.name = kindName;
      input.checked = input.value === kindValue;
    });

    categorySelect = qs('select[data-field="category"]', rowEl);
    if (categorySelect) {
      categorySelect.setAttribute('data-selected', item.category);
      categorySelect.setAttribute('data-categories', item.categories);
    }

    setRowStatus(rowEl, 'pending', '待确认');
    return rowEl;
  }

  function initCategorySelect(rowEl) {
    var select = qs('select[data-field="category"]', rowEl);
    var categoriesInput;
    var selected;
    var options;
    var categoryList;

    if (!select) {
      return;
    }

    categoriesInput = rowField(rowEl, 'categories');
    selected = normalizeImportListValue(select.getAttribute('data-selected'));
    options = splitCSV(select.getAttribute('data-categories'));

    if (selected) {
      options = appendUnique(options, selected);
    }

    baseCategoryOptions.forEach(function(name) {
      options = appendUnique(options, name);
    });

    if (options.length === 0) {
      options.push('');
    }

    if (!selected) {
      selected = options[0];
    }

    select.innerHTML = '';
    options.forEach(function(name) {
      var option = document.createElement('option');
      option.value = name;
      option.textContent = name || '未设置';
      select.appendChild(option);
    });

    select.value = selected;
    if ((select.value || '') !== selected) {
      select.selectedIndex = 0;
      selected = select.value || '';
    }

    categoryList = splitCSV(categoriesInput ? categoriesInput.value : '');
    categoryList = appendUnique(categoryList, selected);
    if (categoriesInput) {
      categoriesInput.value = categoryList.join(', ');
    }

    select.onchange = function() {
      var value = (select.value || '').trim();
      var list = splitCSV(categoriesInput ? categoriesInput.value : '');
      list = appendUnique(list, value);
      if (categoriesInput) {
        categoriesInput.value = list.join(', ');
      }
      select.setAttribute('data-selected', value);
    };
  }

  function normalizeSummaryKind(variant) {
    var key = (variant || '').trim().toLowerCase();
    if (key === 'success') {
      return 'success';
    }
    if (key === 'danger' || key === 'error') {
      return 'danger';
    }
    if (key === 'info') {
      return 'info';
    }
    return 'warning';
  }

  function setEditSummary(text, variant) {
    var kind = normalizeSummaryKind(variant);
    var message = text || '';

    if (editSummaryBody) {
      editSummaryBody.textContent = message;
    } else {
      editSummary.textContent = message;
    }

    editSummary.classList.remove('info', 'success', 'warning', 'danger');
    editSummary.classList.add(kind);
  }

  function formatFailedDetails(items, maxItems) {
    var list = Array.isArray(items) ? items : [];
    var limit = Number(maxItems || 3);
    var text;

    if (list.length === 0) {
      return '';
    }

    if (!Number.isFinite(limit) || limit < 1) {
      limit = 3;
    }

    text = list.slice(0, limit).join('；');
    if (list.length > limit) {
      text += '；...';
    }
    return text;
  }

  function setRetryVisible(rowEl, visible) {
    var button = qs('[data-import-row-retry-btn]', rowEl);
    if (!button) {
      return;
    }
    button.hidden = !visible;
  }

  function setRowStatus(rowEl, state, text, detail, allowRetry) {
    var cell = qs('[data-import-edit-status]', rowEl);
    var label;
    var detailEl;

    if (!cell) {
      return;
    }
    cell.setAttribute('data-state', state);
    label = qs('.import-row-status-label', cell);
    detailEl = qs('.import-row-status-detail', cell);
    if (label) {
      label.textContent = text;
    } else {
      cell.textContent = text;
    }
    if (detailEl) {
      detailEl.textContent = detail || '';
      detailEl.hidden = !detail;
    }
    setRetryVisible(rowEl, !!allowRetry);
  }

  function removeImportRow(rowEl) {
    if (!rowEl) {
      return;
    }
    rowEl.remove();
    importingTotalCount = Math.max(0, importingTotalCount - 1);
    syncEditEmptyState();
  }

  function parseConfirmAllFailure(raw) {
    var text = (raw || '').trim();
    var match;

    if (!text) {
      return null;
    }

    match = text.match(/^ID=(\d+)(?:\((.*?)\))?:\s*(.+)$/);
    if (!match) {
      return null;
    }

    return {
      postID: Number(match[1] || 0),
      title: match[2] || '',
      error: match[3] || text
    };
  }

  function mapConfirmAllFailures(errors) {
    var result = {};

    (Array.isArray(errors) ? errors : []).forEach(function(item) {
      var parsed = parseConfirmAllFailure(item);
      if (!parsed || !parsed.postID) {
        return;
      }
      result[parsed.postID] = parsed;
    });

    return result;
  }

  function applySavedItemToRow(rowEl, rawItem) {
    var item = normalizeItem(rawItem);
    var statusName;
    var statusValue;
    var kindName;
    var kindValue;
    var categorySelect;

    if (!item.post_id) {
      return;
    }

    rowEl.setAttribute('data-post-id', item.post_id);
    rowField(rowEl, 'post_id').value = item.post_id;
    rowField(rowEl, 'filename').value = item.filename;
    rowField(rowEl, 'created_at_unix').value = item.created_at_unix;
    rowField(rowEl, 'title').value = item.title;
    rowField(rowEl, 'slug').value = item.slug;
    rowField(rowEl, 'created_at').value = item.created_at;
    rowField(rowEl, 'tags').value = item.tags;
    rowField(rowEl, 'categories').value = item.categories;

    statusName = 'import_status_' + item.post_id;
    statusValue = item.status === 'published' ? 'published' : 'draft';
    rowFields(rowEl, 'status').forEach(function(input) {
      input.name = statusName;
      input.checked = input.value === statusValue;
    });

    kindName = 'import_kind_' + item.post_id;
    kindValue = item.kind === '1' ? '1' : '0';
    rowFields(rowEl, 'kind').forEach(function(input) {
      input.name = kindName;
      input.checked = input.value === kindValue;
    });

    categorySelect = qs('select[data-field="category"]', rowEl);
    if (categorySelect) {
      categorySelect.setAttribute('data-selected', item.category);
      categorySelect.setAttribute('data-categories', item.categories);
    }
    initCategorySelect(rowEl);
  }

  function ensureEditPanelVisible() {
    if (editPanel.hidden) {
      editPanel.hidden = false;
    }
  }

  function syncEditEmptyState() {
    var hasRows = qsa('tr[data-import-item-row]', editRows).length > 0;
    editTableWrap.hidden = !hasRows;
    editEmptyState.hidden = hasRows;
  }

  function syncEditSummaryByRows(defaultVariant) {
    var pageCount = qsa('tr[data-import-item-row]', editRows).length;
    if (importingTotalCount <= 0) {
      setEditSummary('暂无待确认导入记录', 'success');
      syncEditEmptyState();
      return;
    }
    if (pageCount === 0) {
      syncEditEmptyState();
      setEditSummary('待确认 ' + importingTotalCount + ' 条导入记录（当前页暂无记录）', defaultVariant || 'warning');
      return;
    }
    syncEditEmptyState();
    setEditSummary('待确认 ' + importingTotalCount + ' 条导入记录', defaultVariant || 'warning');
  }

  function clearImportingRows() {
    importingTotalCount = 0;
    qsa('tr[data-import-item-row]', editRows).forEach(function(row) {
      row.remove();
    });
    syncEditEmptyState();
    if (!editPanel.hidden) {
      editPanel.hidden = true;
    }
  }

  function appendImportEditRow(rawItem) {
    var item = normalizeItem(rawItem);
    var newRow;
    var existing;

    if (!item.post_id) {
      return;
    }

    ensureEditPanelVisible();

    newRow = createImportRow(item);
    if (!newRow) {
      setEditSummary('导入预览行渲染失败，请刷新后重试', 'error');
      return;
    }

    existing = qs('tr[data-import-item-row][data-post-id="' + item.post_id + '"]', editRows);
    if (existing) {
      existing.replaceWith(newRow);
    } else {
      editRows.appendChild(newRow);
      importingTotalCount += 1;
    }

    initCategorySelect(newRow);
    syncEditSummaryByRows('warning');
  }

  function collectOptions() {
    return {
      title_source: readControlValue('select[name="title_source"]', 'frontmatter'),
      title_field: readControlValue('input[name="title_field"]', 'title'),
      title_level: readControlValue('select[name="title_level"]', '1'),
      slug_source: readControlValue('select[name="slug_source"]', 'filename'),
      slug_field: readControlValue('input[name="slug_field"]', 'slug'),
      created_source: readControlValue('select[name="created_source"]', 'frontmatter'),
      created_field: readControlValue('input[name="created_field"]', 'date'),
      status_source: readControlValue('select[name="status_source"]', 'frontmatter'),
      status_field: readControlValue('input[name="status_field"]', 'draft'),
      category_source: readControlValue('select[name="category_source"]', 'none'),
      category_field: readControlValue('input[name="category_field"]', 'categories'),
      tag_source: readControlValue('select[name="tag_source"]', 'none'),
      tag_field: readControlValue('input[name="tag_field"]', 'tags')
    };
  }

  function buildParseFormData(file, options) {
    var formData = new FormData();
    formData.append('file', file, file.name);
    Object.keys(options).forEach(function(key) {
      formData.append(key, options[key]);
    });
    return formData;
  }

  function readField(rowEl, fieldName) {
    var field = rowField(rowEl, fieldName);
    return ((field && field.value) || '').trim();
  }

  function readCheckedValue(rowEl, fieldName, fallbackValue) {
    var value = qs('[data-field="' + fieldName + '"]:checked', rowEl);
    if (!value || value.value == null || value.value === '') {
      return fallbackValue || '';
    }
    return value.value;
  }

  function buildConfirmPayload(rowEl) {
    var params = new URLSearchParams();
    var category = readField(rowEl, 'category');
    var categories = readField(rowEl, 'categories');

    if (!categories && category) {
      categories = category;
    }

    params.set('post_id', readField(rowEl, 'post_id'));
    params.set('filename', readField(rowEl, 'filename'));
    params.set('title', readField(rowEl, 'title'));
    params.set('slug', readField(rowEl, 'slug'));
    params.set('status', readCheckedValue(rowEl, 'status', 'draft'));
    params.set('kind', readCheckedValue(rowEl, 'kind', '0'));
    params.set('created_at', readField(rowEl, 'created_at'));
    params.set('created_at_unix', readField(rowEl, 'created_at_unix'));
    params.set('tags', readField(rowEl, 'tags'));
    params.set('category', category);
    params.set('categories', categories);

    return params;
  }

  async function saveImportRow(rowEl, options) {
    var opts = options || {};
    var payload;
    var ret;
    var result;
    var errorMessage;
    var requestErrMsg;

    if (!rowEl) {
      return false;
    }

    activeSaveRequests += 1;
    setRowStatus(rowEl, 'running', opts.runningText || '保存中...');
    setDisabled(rowEl, true);
    payload = buildConfirmPayload(rowEl);

    try {
      ret = await sfetchJSON(saveItemURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8'
        },
        body: payload.toString()
      });

      result = ret.body;
      if (!ret.ok || !result || !result.ok) {
        errorMessage = (result && result.error) ? result.error : ('HTTP ' + ret.status);
        setRowStatus(rowEl, 'save-error', '保存失败', errorMessage);
        if (opts.notify !== false) {
          notify('保存失败：' + errorMessage, '', { variant: 'danger' });
        }
        return false;
      }

      if (result.item) {
        applySavedItemToRow(rowEl, result.item);
      }
      setRowStatus(rowEl, 'saved', opts.successText || '已保存', opts.successDetail || '待确认导入');
      if (opts.summaryText) {
        setEditSummary(opts.summaryText, opts.summaryVariant || 'warning');
      }
      return true;
    } catch (err) {
      requestErrMsg = (err && err.message) ? err.message : '请求失败';
      setRowStatus(rowEl, 'save-error', '保存失败', requestErrMsg);
      if (opts.notify !== false) {
        notify('保存失败：' + requestErrMsg, '', { variant: 'danger' });
      }
      return false;
    } finally {
      activeSaveRequests = Math.max(0, activeSaveRequests - 1);
      if (!opts.keepDisabled) {
        setDisabled(rowEl, false);
      }
    }
  }

  function markRowAsDirty(rowEl) {
    var statusCell;

    if (!rowEl) {
      return;
    }

    statusCell = qs('[data-import-edit-status]', rowEl);
    if (statusCell && (statusCell.getAttribute('data-state') || '') === 'running') {
      return;
    }
    setRowStatus(rowEl, 'pending', '已修改，待保存');
  }

  function bindSourceToggle(selector, handler) {
    var element = qs(selector, form);
    if (!element) {
      return;
    }

    function sync() {
      handler(element.value);
    }

    element.addEventListener('change', sync);
    sync();
  }

  bindSourceToggle('select[name="title_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#title_field_container');
      hide('#title_level_container');
      return;
    }
    if (value === 'markdown') {
      hide('#title_field_container');
      show('#title_level_container');
      return;
    }
    hide('#title_field_container');
    hide('#title_level_container');
  });

  bindSourceToggle('select[name="slug_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#slug_field_container');
      return;
    }
    hide('#slug_field_container');
  });

  bindSourceToggle('select[name="created_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#created_field_container');
      return;
    }
    hide('#created_field_container');
  });

  bindSourceToggle('select[name="status_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#status_field_container');
      return;
    }
    hide('#status_field_container');
  });

  bindSourceToggle('select[name="category_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#category_field_container');
      return;
    }
    hide('#category_field_container');
  });

  bindSourceToggle('select[name="tag_source"]', function(value) {
    if (value === 'frontmatter') {
      show('#tag_field_container');
      return;
    }
    hide('#tag_field_container');
  });

  fileInput.addEventListener('change', function() {
    setSelectedFiles(fileInput.files);
  });

  toggleImportStatusBtn.addEventListener('click', function() {
    var rows = qsa('tr[data-import-item-row]', editRows);
    var allDraft = true;

    if (rows.length === 0) {
      return;
    }

    rows.forEach(function(rowEl) {
      var current = readCheckedValue(rowEl, 'status', 'draft');
      if (current !== 'draft') {
        allDraft = false;
      }
    });

    rows.forEach(function(rowEl) {
      var targetStatus = allDraft ? 'published' : 'draft';
      var target = qs('[data-field="status"][value="' + targetStatus + '"]', rowEl);
      if (target) {
        target.checked = true;
      }
    });
  });

  function handleRowFieldMutation(event) {
    if (!event.target.matches('[data-field="title"], [data-field="slug"], [data-field="created_at"], [data-field="tags"], [data-field="categories"], [data-field="category"], [data-field="status"], [data-field="kind"]')) {
      return;
    }
    markRowAsDirty(event.target.closest('tr[data-import-item-row]'));
  }

  editRows.addEventListener('input', handleRowFieldMutation);
  editRows.addEventListener('change', handleRowFieldMutation);

  editRows.addEventListener('click', async function(event) {
    var button = event.target.closest('[data-import-row-save-btn]');
    var rowEl;
    var saved;

    if (!button) {
      return;
    }
    if (isParsing || isConfirming || isCancelling) {
      return;
    }

    rowEl = button.closest('tr[data-import-item-row]');
    if (!rowEl || button.disabled) {
      return;
    }

    saved = await saveImportRow(rowEl, {
      summaryText: '单行保存成功，可继续编辑或直接确认导入全部记录。',
      summaryVariant: 'warning'
    });
    if (!saved) {
      return;
    }
  });

  editRows.addEventListener('click', async function(event) {
    var button = event.target.closest('[data-import-row-retry-btn]');
    var rowEl;
    var payload;
    var result;

    if (!button) {
      return;
    }
    if (isParsing || isConfirming || isCancelling || button.disabled) {
      return;
    }

    rowEl = button.closest('tr[data-import-item-row]');
    if (!rowEl) {
      return;
    }

    payload = buildConfirmPayload(rowEl);
    setDisabled(rowEl, true);
    setRowStatus(rowEl, 'running', '重试中...');

    try {
      var ret = await sfetchJSON(confirmItemURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8'
        },
        body: payload.toString()
      });

      result = ret.body;
      if (!ret.ok || !result || !result.ok) {
        var errorMessage = (result && result.error) ? result.error : ('HTTP ' + ret.status);
        setRowStatus(rowEl, 'confirm-error', '导入失败', errorMessage, true);
        notify('重试失败：' + errorMessage, '', { variant: 'danger' });
        return;
      }

      removeImportRow(rowEl);
      if (importingTotalCount <= 0) {
        editPanel.hidden = true;
        setEditSummary('所有待确认导入记录已完成。', 'success');
      } else {
        setEditSummary('重试成功，剩余待确认 ' + importingTotalCount + ' 条。', 'warning');
      }
      notify('重试成功，已完成该条导入。', '', { variant: 'success' });
    } catch (err) {
      var requestErrMsg = (err && err.message) ? err.message : '请求失败';
      setRowStatus(rowEl, 'confirm-error', '导入失败', requestErrMsg, true);
      notify('重试失败：' + requestErrMsg, '', { variant: 'danger' });
    } finally {
      setDisabled(rowEl, false);
    }
  });

  async function executeCancelImporting() {
    if (isParsing || isConfirming || isCancelling) {
      return;
    }

    isCancelling = true;
    uploadRoot.classList.add('is-disabled');
    setDisabled(editPanel, true);
    cancelConfirmBtn.disabled = true;

    try {
      var cancelPayload = new URLSearchParams();
      var ret = await sfetchJSON(cancelItemURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8'
        },
        body: cancelPayload.toString()
      });
      var result = ret.body;

      if (!ret.ok || !result || !result.ok) {
        var errorMessage = (result && result.error) ? result.error : ('HTTP ' + ret.status);
        setEditSummary('取消导入失败：' + errorMessage, 'error');
        notify('取消导入失败：' + errorMessage, '', { variant: 'danger' });
        return;
      }

      var deletedCount = Number(result.deleted_count || 0);
      if (!Number.isFinite(deletedCount) || deletedCount < 0) {
        deletedCount = 0;
      }

      clearImportingRows();
      setEditSummary('已取消导入，删除 ' + deletedCount + ' 条临时记录。', 'success');
      notify('已取消导入，删除 ' + deletedCount + ' 条临时记录。', '', { variant: 'success' });
      goTo(buildImportPageURL(1));
      return;
    } catch (err) {
      var requestErrMsg = (err && err.message) ? err.message : '请求失败';
      setEditSummary('取消导入失败：' + requestErrMsg, 'error');
      notify('取消导入失败：' + requestErrMsg, '', { variant: 'danger' });
    } finally {
      isCancelling = false;
      setDisabled(editPanel, false);
      cancelConfirmBtn.disabled = false;
      if (!isParsing && !isConfirming) {
        uploadRoot.classList.remove('is-disabled');
      }
    }
  }

  cancelBtn.addEventListener('click', async function() {
    var confirmed = false;
    var confirmAPI;

    if (isParsing || isConfirming || isCancelling) {
      return;
    }

    confirmAPI = window.DashAppUI.confirm;
    confirmed = await confirmAPI.ask({
        dialogId: 'import-cancel-dialog',
        title: '确认取消',
        message: '将删除所有“导入中”状态的临时文章，且无法恢复。确定继续吗？',
        opener: this,
        okSelector: '#import-cancel-confirm-btn',
      });

    if (!confirmed) {
      return;
    }
    void executeCancelImporting();
  });

  confirmBtn.addEventListener('click', async function() {
    var total;
    var successCount;
    var failCount;
    var failDetail;
    var failMessage;
    var visibleRows;
    var i;
    var saved;

    if (isConfirming || isCancelling) {
      return;
    }

    if (activeSaveRequests > 0) {
      setEditSummary('有记录正在保存，请稍后再试。', 'warning');
      return;
    }

    if (importingTotalCount <= 0) {
      setEditSummary('暂无待确认导入记录', 'success');
      return;
    }

    isConfirming = true;
    uploadRoot.classList.add('is-disabled');
    setDisabled(editPanel, true);

    try {
      visibleRows = qsa('tr[data-import-item-row]', editRows);
      if (visibleRows.length > 0) {
        setEditSummary('正在保存当前页修改...', 'warning');
      }
      for (i = 0; i < visibleRows.length; i++) {
        saved = await saveImportRow(visibleRows[i], {
          notify: false,
          keepDisabled: true,
          successDetail: '待确认导入'
        });
        if (!saved) {
          setEditSummary('确认导入前自动保存失败，请先修正当前页错误后重试。', 'error');
          notify('确认导入前自动保存失败，请先修正当前页错误后重试。', '', { variant: 'danger' });
          return;
        }
      }

      visibleRows.forEach(function(rowEl) {
        setRowStatus(rowEl, 'running', '确认中...');
      });
      setEditSummary('正在确认导入全部待确认记录...', 'warning');
      var ret = await sfetchJSON(confirmAllURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8'
        },
        body: ''
      });
      var result = ret.body;

      if (!ret.ok || !result || !result.ok) {
        var errorMessage = (result && result.error) ? result.error : ('HTTP ' + ret.status);
        setEditSummary('确认导入失败：' + errorMessage, 'error');
        notify('确认导入失败：' + errorMessage, '', { variant: 'danger' });
        return;
      }

      total = Number(result.total || 0);
      if (!Number.isFinite(total) || total < 0) {
        total = 0;
      }

      successCount = Number(result.success || 0);
      if (!Number.isFinite(successCount) || successCount < 0) {
        successCount = 0;
      }

      failCount = Number(result.fail || 0);
      if (!Number.isFinite(failCount) || failCount < 0) {
        failCount = 0;
      }

      if (total === 0) {
        setEditSummary('暂无待确认导入记录', 'success');
        return;
      }

      var failuresByID = mapConfirmAllFailures(result.errors || []);
      var visibleRows = qsa('tr[data-import-item-row]', editRows);
      var visibleFailureCount = 0;

      visibleRows.forEach(function(rowEl) {
        var postID = Number(readField(rowEl, 'post_id') || 0);
        var failure = failuresByID[postID];

        if (failure) {
          visibleFailureCount += 1;
          setRowStatus(rowEl, 'confirm-error', '导入失败', failure.error, true);
          return;
        }

        removeImportRow(rowEl);
      });

      if (failCount === 0) {
        var doneMessage = '确认完成：共 ' + total + ' 条，成功导入 ' + successCount + ' 条。';
        if (importingTotalCount <= 0) {
          editPanel.hidden = true;
        }
        setEditSummary(doneMessage, 'success');
        notify(doneMessage, '', { variant: 'success' });
        return;
      }

      failDetail = formatFailedDetails(result.errors || [], 2);
      failMessage = '确认完成：成功 ' + successCount + ' 条，失败 ' + failCount + ' 条。';
      if (failDetail) {
        failMessage += ' 失败详情：' + failDetail;
      }
      if (visibleFailureCount === 0 && importingTotalCount > 0) {
        failMessage += ' 当前页无失败记录，请刷新或切页查看剩余失败项。';
      }
      setEditSummary(failMessage, 'error');
      notify(failMessage, '', { variant: 'danger' });
      return;
    } catch (err) {
      var requestErrMsg = (err && err.message) ? err.message : '请求失败';
      setEditSummary('确认导入失败：' + requestErrMsg, 'error');
      notify('确认导入失败：' + requestErrMsg, '', { variant: 'danger' });
    } finally {
      setDisabled(editPanel, false);
      isConfirming = false;
      if (!isParsing && !isCancelling) {
        uploadRoot.classList.remove('is-disabled');
      }
    }
  });

  form.addEventListener('submit', async function(event) {
    var files;
    var options;
    var successCount = 0;
    var failCount = 0;
    var completedCount = 0;
    var failedNames = [];
    var detail;
    var message;

    event.preventDefault();
    if (isParsing || isCancelling || isConfirming) {
      return;
    }

    files = selectedFiles.slice();
    if (files.length === 0) {
      files = selectedFilesFromInput();
    }

    if (files.length === 0) {
      setEditSummary('请至少选择一个文件', 'error');
      return;
    }

    options = collectOptions();
    isParsing = true;
    uploadRoot.classList.add('is-disabled');
    ensureEditPanelVisible();
    setEditSummary('开始逐个导入文件（临时状态：导入中）...', 'warning');
    setDisabled(form, true);

    await runWithConcurrency(files, parseConcurrency, async function(file) {
      var formData = buildParseFormData(file, options);
      var ret;
      var result;
      var errMsg;

      try {
        ret = await sfetchJSON(parseItemURL, {
          method: 'POST',
          body: formData
        });
        result = ret.body;

        if (ret.ok && result && result.ok) {
          successCount += 1;
        } else {
          failCount += 1;
          errMsg = (result && result.error) ? result.error : ('HTTP ' + ret.status);
          failedNames.push((file.name || '未命名文件') + '：' + errMsg);
        }
      } catch (err) {
        failCount += 1;
        failedNames.push((file.name || '未命名文件') + '：' + (err && err.message ? err.message : '请求失败'));
      }

      completedCount += 1;
      setEditSummary(
        '导入进度 ' + completedCount + '/' + files.length + '，成功 ' + successCount + '，失败 ' + failCount,
        failCount > 0 ? 'error' : 'warning'
      );
    });

    setDisabled(form, false);
    isParsing = false;
    if (!isConfirming && !isCancelling) {
      uploadRoot.classList.remove('is-disabled');
    }

    if (successCount > 0 && failCount === 0) {
      goTo(buildImportPageURL(1));
      return;
    }

    if (successCount > 0) {
      detail = formatFailedDetails(failedNames, 2);
      message = '导入完成：成功 ' + successCount + '，失败 ' + failCount + '。';
      if (detail) {
        message += '失败详情：' + detail;
      }
      setEditSummary(message + ' 即将刷新导入列表。', 'error');
      goTo(buildImportPageURL(1));
      return;
    }

    detail = formatFailedDetails(failedNames, 3);
    if (detail) {
      setEditSummary('导入失败：' + detail, 'error');
      return;
    }
    setEditSummary('导入失败，请检查文件或配置。', 'error');
  });

  qsa('tr[data-import-item-row]', editRows).forEach(function(rowEl) {
    initCategorySelect(rowEl);
  });
  selectedFiles = selectedFilesFromInput();
  syncEditSummaryByRows('warning');
});
