// Lo único que hace falta JavaScript aquí es copiar el magnet al portapapeles.
// Todo lo demás lo pinta el servidor.
//
// El magnet no viaja dentro del HTML porque no se sabe hasta que se abre la
// ficha del sitio, y abrir catorce fichas para una búsqueda que el usuario va a
// usar una vez sería castigar al sitio sin motivo. Así que se pide aquí, solo
// del resultado que se pincha.
document.addEventListener("click", async (evento) => {
  const boton = evento.target.closest(".boton--copiar");
  if (!boton) return;

  const original = boton.textContent;
  boton.disabled = true;
  boton.textContent = "Copiando…";

  try {
    const respuesta = await fetch(boton.dataset.magnet + "&formato=texto");
    if (!respuesta.ok) throw new Error(respuesta.statusText);

    const magnet = (await respuesta.text()).trim();
    await navigator.clipboard.writeText(magnet);

    boton.textContent = "Copiado";
  } catch (err) {
    // Sin portapapeles (o sin HTTPS) no hay nada que hacer salvo decirlo.
    boton.textContent = "No se pudo";
    console.error("copiando el magnet:", err);
  } finally {
    setTimeout(() => {
      boton.textContent = original;
      boton.disabled = false;
    }, 1500);
  }
});
