// JavaScript hace aquí dos cosas: copiar el magnet al portapapeles y ampliar la
// carátula. Todo lo demás lo pinta el servidor.
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

// El visor de carátulas. Hay un solo <dialog> en la página y se rellena con lo
// que trae el botón pinchado, que ya lo escribió el servidor: así no hay que
// pedir nada al abrirlo ni repetir setenta diálogos en el HTML.
//
// La imagen grande no se descarga hasta que se pincha. Por eso la lista sigue
// pesando lo mismo que antes de que esto existiera.
document.addEventListener("click", (evento) => {
  const visor = document.getElementById("visor");
  if (!visor) return;

  const boton = evento.target.closest(".cartel-abrir");
  if (boton) {
    const d = boton.dataset;

    visor.querySelector(".visor-cartel").src = d.grande;
    visor.querySelector(".visor-cartel").alt = "Carátula de " + d.obra;
    visor.querySelector(".visor-titulo").textContent = d.titulo;

    // El punto solo separa si hay algo a los dos lados.
    const meta = [d.generos, d.nota ? "★ " + d.nota : ""].filter(Boolean);
    visor.querySelector(".visor-meta").textContent = meta.join(" · ");
    visor.querySelector(".visor-sinopsis").textContent = d.sinopsis || "";

    visor.showModal();
    return;
  }

  // Pinchar fuera cierra. El propio <dialog> ocupa toda la pantalla —el fondo
  // oscuro es él—, así que "fuera" es pinchar en él pero no en su contenido.
  if (visor.open && evento.target === visor) visor.close();
});
