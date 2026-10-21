function validPhoto(value) {
  return typeof value === 'string' && value.length <= 700000 && /^data:image\/(jpeg|png);base64,[A-Za-z0-9+/]+=*$/.test(value);
}
function createComposer(text, report, changed = () => {}) {
  const root = text.closest('form');
  const controls = document.createElement('div'); controls.className = 'capture-tools';
  const preview = document.createElement('img'); preview.className = 'attached-photo'; preview.alt = 'Attached photo'; preview.hidden = true;
  let photo = '', loading = false, locked = false, recognition;
  const buttons = [];
  const sync = () => { buttons.forEach(b => b.disabled = locked || loading); };
  const setImage = value => { photo = validPhoto(value) ? value : ''; preview.hidden = !photo; if(photo) preview.src = photo; else preview.removeAttribute('src'); remove.hidden = !photo; changed(photo); };
  for (const label of ['Photo', 'Camera']) {
    const input = document.createElement('input'); input.type = 'file'; input.accept = 'image/*'; input.hidden = true;
    if(label === 'Camera') input.setAttribute('capture', 'environment');
    const button = document.createElement('button'); button.type = 'button'; button.textContent = label; buttons.push(button);
    button.onclick = () => input.click();
    input.onchange = async () => {
      const file = input.files[0]; if(!file) return;
      loading = true; sync();
      try {
        if(file.size > 25*1024*1024) throw Error('Choose a photo smaller than 25 MB.');
        const bitmap = await createImageBitmap(file);
        const canvas = document.createElement('canvas'); const scale = Math.min(1, 1600/Math.max(bitmap.width, bitmap.height));
        canvas.width = Math.max(1, Math.round(bitmap.width*scale)); canvas.height = Math.max(1, Math.round(bitmap.height*scale));
        const ctx = canvas.getContext('2d'); ctx.fillStyle = '#fff'; ctx.fillRect(0,0,canvas.width,canvas.height); ctx.drawImage(bitmap,0,0,canvas.width,canvas.height); bitmap.close();
        let result;
        for(let quality = .85; quality >= .25; quality -= .1) { result = canvas.toDataURL('image/jpeg', quality); if(result.length <= 699000) break; }
        if(result.length > 699000) throw Error('This photo is too large. Choose a smaller photo.');
        setImage(result); report('');
      } catch(error) { report(error.message || 'Could not open that photo. Try JPEG or PNG.'); }
      finally { loading = false; sync(); input.value = ''; }
    };
    controls.append(button, input);
  }
  const talk = document.createElement('button'); talk.type = 'button'; talk.textContent = 'Talk'; buttons.push(talk);
  talk.onclick = () => {
    if(recognition) { recognition.stop(); return; }
    const Speech = window.SpeechRecognition || window.webkitSpeechRecognition;
    if(!Speech) { report('Dictation is not supported in this browser.'); return; }
    recognition = new Speech(); talk.textContent = 'Stop';
    recognition.onresult = event => { text.value += (text.value ? ' ' : '') + event.results[0][0].transcript; text.dispatchEvent(new Event('input')); };
    recognition.onerror = () => report('Could not hear you. Try again or type.');
    recognition.onend = () => { recognition = null; talk.textContent = 'Talk'; };
    try { recognition.start(); } catch(_) { recognition = null; talk.textContent = 'Talk'; report('Could not start dictation.'); }
  };
  const remove = document.createElement('button'); remove.type = 'button'; remove.textContent = 'Remove photo'; remove.hidden = true; remove.onclick = () => setImage(''); buttons.push(remove);
  controls.append(talk, remove); text.after(preview, controls);
  window.addEventListener('pagehide', () => { if(recognition) recognition.abort(); });
  return {get image(){return photo;}, get loading(){return loading;}, setImage, setBusy(value){locked=value; if(value && recognition) recognition.stop(); sync();}};
}
