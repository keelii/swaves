  (function () {
    var root = document.getElementById("comments");
    if (!root) return;

    var parentInput = document.getElementById("comment-parent-id");
    var replyState = document.getElementById("comment-reply-state");
    var replyAuthor = document.getElementById("comment-reply-author");
    var cancelBtn = document.getElementById("comment-reply-cancel");

    function resetReply() {
      parentInput.value = "0";
      replyAuthor.textContent = "";
      if (replyState) {
        replyState.hidden = true;
      }
    }

    root.addEventListener("click", function (event) {
      var trigger = event.target.closest("[data-comment-reply]");
      if (!trigger) return;
      event.preventDefault();

      var commentID = trigger.getAttribute("data-comment-id");
      var author = trigger.getAttribute("data-comment-author") || "";
      if (!commentID) return;

      parentInput.value = commentID;
      replyAuthor.textContent = author;
      if (replyState) {
        replyState.hidden = false;
      }
      document.getElementById("comment-content").focus();
    });

    if (cancelBtn) {
      cancelBtn.addEventListener("click", function () {
        resetReply();
      });
    }
  })();
