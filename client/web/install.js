(function () {
  var button = document.getElementById('install');
  var installPrompt;
  if (!button) return;

  window.addEventListener('beforeinstallprompt', function (event) {
    event.preventDefault();
    installPrompt = event;
    button.hidden = false;
  });

  button.addEventListener('click', async function () {
    if (!installPrompt) return;
    var prompt = installPrompt;
    installPrompt = null;
    button.hidden = true;
    try {
      await prompt.prompt();
      await prompt.userChoice;
    } catch (error) {
      console.warn('Unable to open the install prompt', error);
    }
  });

  window.addEventListener('appinstalled', function () {
    installPrompt = null;
    button.hidden = true;
  });
}());
