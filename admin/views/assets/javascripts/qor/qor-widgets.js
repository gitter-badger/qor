(function (factory) {
  if (typeof define === 'function' && define.amd) {
    // AMD. Register as anonymous module.
    define('qor-widgets', ['jquery'], factory);
  } else if (typeof exports === 'object') {
    // Node / CommonJS
    factory(require('jquery'));
  } else {
    // Browser globals.
    factory(jQuery);
  }
})(function ($) {

  'use strict';

  var Widgets = {};

  Widgets.init = function () {
    this.confirm();
  };

  Widgets.confirm = function () {
    $(document).on('click.qor.confirmer', '[data-confirm]', function (e) {
      var $this = $(this),
          data = $this.data(),
          url;

      if (data.confirm) {
        if (window.confirm(data.confirm)) {
          if (/DELETE/i.test(data.method)) {
            e.preventDefault();

            url = data.url || $this.attr('href');
            data = $.extend({}, data, {
              _method: 'DELETE'
            });

            $.post(url, data, function () {
              window.location.reload();
            });

          }
        } else {
          e.preventDefault();
        }
      }
    });
  };

  $(function () {
    Widgets.init();
  });

  return Widgets;

});
