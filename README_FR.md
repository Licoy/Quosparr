[English](./README.md) · [简体中文](./README_ZH.md) · [繁體中文](./README_ZH_HANT.md) · [한국어](./README_KO.md) · [日本語](./README_JA.md) · [Русский](./README_RU.md) · Français

<div align="center">

<img src="Resources/AppIcon.png" alt="Logo Quosparr" width="128" height="128">

# Quosparr

<sub><i>Nom formé de <b>Quo</b>ta (quota) + <b>Sparr</b>ow (moineau)</i></sub>

Un petit moineau perché dans votre barre de menus, qui veille sur vos quotas Codex, Claude et Grok.

Quosparr est une application native de barre de menus / zone de notification pour macOS, Windows et Linux. Elle affiche les quotas Codex, d’abonnement Claude Code et Grok, les reset credits, le coût équivalent API et les sessions locales, avec une gestion des comptes séparée pour chaque fournisseur. Les skills globaux, les serveurs MCP et les sessions locales des trois fournisseurs se gèrent au même endroit.

</div>

## Points forts

**Codex**

- Quota restant pour les fenêtres de 5 heures, hebdomadaire et supplémentaires, ainsi que le compte actuel, le niveau d’abonnement et l’expiration.
- Nombre de crédits de réinitialisation, statut et heure d’expiration.
- Estimation de l’allocation hebdomadaire à partir du pourcentage hebdomadaire officiel et des Credits journaliers, comparée aux semaines précédentes pour repérer une baisse de quota.
- Actualités des réinitialisations Codex : savoir si les limites Codex ont été réinitialisées aujourd’hui, avec son propre intervalle d’actualisation (données de [Did Codex Reset](https://didcodexreset.com)).
- Comptes fournisseurs : connexion navigateur, import du `auth.json` local, collage de jeton / JSON (y compris `/auth/session`), import de fichiers ou ajout d’une clé API. Le basculement, après confirmation, écrit atomiquement `~/.codex/auth.json`.
- Comptes API : URL de base + clé API (URL de base vide : API OpenAI officielle), avec un modèle facultatif choisi dans la liste du site. La bascule écrit `auth.json` et le fournisseur `quosparr` dans `config.toml` (sauvegardés avant), et le solde des relais de type new-api ou compatibles OpenAI s’affiche dans la liste des comptes, le panneau, la barre d’état, la fenêtre flottante et le widget. Une connexion par clé dont `config.toml` sélectionne déjà un relais est reprise avec ce site et ce modèle. Modifier le compte utilisé permet de remplacer sa clé (vide : la conserver) et demande s’il faut réécrire les fichiers de configuration tout de suite ; redémarrez ensuite Codex.
- Sessions récentes, projets et statut, ainsi que la réparation de l’index de session local.

**Claude**

- Usage d’abonnement Claude Code : fenêtre de 5 heures, fenêtre de 7 jours, fenêtres hebdomadaires par modèle et usage supplémentaire. Cette lecture n’est pas une API de facturation publique documentée.
- Les journaux de session locaux de Claude Code donnent l’usage de jetons, le coût équivalent API (`anthropic-builtin-2026-10-14`), les sessions récentes et la vitesse de sortie. Une même réponse n’est comptée qu’une fois. Les modèles inconnus n’ont pas de coût exact. Les journaux n’ont pas l’instant du premier jeton : cette colonne affiche « — » et la vitesse utilise la durée de bout en bout.
- Comptes d’abonnement : connexion par navigateur sans lancer le CLI au préalable, import de la connexion actuelle, collage des identifiants ou import d’un fichier. La connexion par navigateur permet l’annulation et le collage du rappel, ajoute seulement le compte à la bibliothèque et exige un basculement explicite pour l’utiliser dans Claude Code. Quosparr n’actualise pas le compte actuel ; si la connexion expire, lancez `claude` une fois. Les autres comptes gérés ne sont actualisés que dans leurs copies de bibliothèque.
- Sur macOS, l’identifiant officiel reste dans le trousseau et passe par `/usr/bin/security`. Sous Windows et Linux, c’est `<config>/.credentials.json`. Un basculement ne remplace que `claudeAiOauth`, conserve les jetons MCP et ne met à jour que `oauthAccount` dans `.claude.json`. Un processus Claude Code en cours n’est pas arrêté et peut réécrire l’ancienne connexion ; l’app prévient avant le basculement.
- Comptes API : URL de base + clé API, avec une correspondance facultative des modèles Opus / Sonnet / Haiku / Fable tirée de la liste du site. La bascule n’écrit que `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` et la correspondance dans `settings.json` (sauvegardé avant) sans toucher à la connexion par abonnement ; revenir à l’abonnement retire ces clés. Le solde du relais s’affiche partout où apparaît la consommation Claude. Un relais déjà défini dans `settings.json` est importé automatiquement. Modifier le compte utilisé permet de remplacer sa clé (vide : la conserver) et demande s’il faut réécrire `settings.json` tout de suite ; redémarrez ensuite Claude Code.

- **Abonnements Claude Desktop** : associez un compte depuis l’une des deux listes, connectez-vous manuellement au compte et à l’organisation affichés, puis confirmez dans Quosparr. Chaque opération demande d’enregistrer vos tâches et d’accepter la fermeture de Desktop et l’enregistrement/restauration de sa configuration locale complète, connexion, réglages et état local. Terminer ou annuler l’association restaure la configuration précédente. Le changement lié est désactivé par défaut ; le compte CLI actuel peut aussi ouvrir son Desktop associé indépendamment. Les résultats CLI et Desktop sont séparés ; le lancement ne vérifie pas l’identité.
- Les profils Desktop et les données de connexion chiffrées nativement restent privés dans `~/.quosparr/claude-desktop` (ACL protégées réservées à l’utilisateur sous Windows), conservent le chemin officiel et ne sont ni déchiffrés ni exportés avec les comptes. La configuration d’origine est conservée séparément sans importer son compte ; aucune donnée de navigateur ou d’autre application n’est lue. Les profils doivent partager le volume des données Desktop ; une restauration explicite est disponible après interruption. Les comptes API sont exclus. La validation sur appareils réels et de compatibilité des versions Desktop sous macOS, Windows et Linux reste à effectuer.

**Grok**

- Quota inclus, répartition Build / Imagine / Chat, périodes hebdomadaires / mensuelles, solde prépayé et usage à la demande via l’API de facturation officielle CLI chat-proxy.
- Plusieurs comptes OAuth / SuperGrok avec connexion isolée, import de la connexion actuelle, collage d’identifiants, actualisation, alias, ordre et basculement explicite.
- Sessions récentes issues des journaux locaux du CLI Grok.

**Tous les fournisseurs**

- Réglages › Prix des modèles se trouve juste avant Avancé, dans le même groupe ; la synchronisation automatique du catalogue public models.dev est activée toutes les heures par défaut, peut être désactivée et propose des intervalles prédéfinis ou personnalisés de 15 minutes à 7 jours, enregistrés immédiatement ; la synchronisation manuelle reste disponible
- Les prix sont des tarifs de référence en USD par million de tokens, sans couvrir toutes les modalités comme les images et vidéos ; référence du catalogue et tarif appliqué peuvent différer pour préserver les paliers de contexte, TTL d’écriture du cache, Fast et règles régionales vérifiés ; les détails indiquent source et version, les tables intégrées datées ci-dessous restant la base hors ligne；Seules les entrées OpenAI, Anthropic et xAI sous leurs identifiants de fournisseur d’origine et leurs tarifs intégrés sont affichés dans les onglets fixes Codex / Claude / Grok, avec Codex par défaut et sans autre fournisseur ; les relais tiers et modèles homonymes des autres fournisseurs sont exclus, models.dev étant un catalogue public agrégé et non une interface officielle en temps réel；Le bouton de réglages à côté des onglets ouvre une fenêtre native ; synchronisation automatique, fréquence et état détaillé sont communs aux trois fournisseurs et enregistrés immédiatement, fermer la fenêtre ne coupe pas la synchronisation
- Le coût équivalent API réestime les usages locaux historiques aux tarifs actuels ; les coûts partiels sont signalés pour le total, les jours, projets, modèles et sessions, avec la liste complète des tarifs ou règles manquants ; — signifie un prix absent, $0 un prix explicitement gratuit ; les quotas d’abonnement et reset credits gardent leur sens
- La synchronisation télécharge uniquement le catalogue public via la politique de proxy enregistrée, sans envoyer les journaux de session ; hors ligne, les prix intégrés et le dernier catalogue enregistré avec succès restent disponibles, avec des erreurs de synchronisation visibles
- Onglets `Codex | Claude | Grok` toujours visibles. Sur macOS, Réglages › Élément de la barre de menus présente un aperçu de cinq styles : barres de quota, compte à rebours, batterie (horizontale ou verticale), anneaux de quota (valeurs éventuellement à l’intérieur) et texte de progression. La barre de menus suit le panneau ou affiche n’importe lesquels des trois fournisseurs, chacun à part ; les barres, comptes à rebours et batteries peuvent aussi empiler deux fournisseurs dans un même élément. Un widget ou un choix déjà enregistré « Codex + Grok » conserve cette paire.
- Une fenêtre flottante facultative, toujours au premier plan, affiche le quota restant d’au plus trois fournisseurs dans quatre styles : capsule, carte, anneaux ou batterie. Un clic ouvre le panneau, un glisser la déplace et un clic droit affiche plus d’options. L’opacité, le verrouillage de la position et l’affichage au-dessus des apps en plein écran sont réglables.
- Coût équivalent API et usage de jetons pour aujourd’hui, le cycle actuel, le cycle précédent, ce mois-ci ou une plage personnalisée.
- Graphique d’usage des jetons depuis le début de l’année, en carte de chaleur, en ligne ou en barres (quotidien / hebdomadaire / cumulé).
- Page Skills pour les skills globaux de chaque fournisseur : leur origine, leur chargement par la CLI et les autres CLI qui les partagent ; recherche, lecture de `SKILL.md`, ouverture du dossier, activation ou désactivation dans la configuration propre à la CLI (sauvegardée avant écriture), copie ou déplacement vers une autre CLI (sans jamais écraser un skill du même nom), mise à la corbeille de skills de Quosparr et repérage des liens cassés, des dossiers imbriqués ou des noms en double, la plupart réparables en un clic ; plusieurs skills peuvent être sélectionnés pour être supprimés ou corrigés ensemble, toujours après affichage des risques et confirmation. Réglages › Skills propose un onglet par fournisseur avec filtres d'état et de type, une page par skill (source d'installation, fichiers, frontmatter, liens vers le dépôt GitHub ou la page d'accueil) et une corbeille d'où restaurer un skill ou l'envoyer dans la corbeille du système. Réglages › Skills permet aussi d’ajouter des skills depuis un dépôt GitHub public (URL ou `owner/repo`), un dossier local ou un zip : Quosparr trouve les dossiers de skills à toute profondeur, laisse de côté le contrôle de version, les sorties de build et les fichiers système, vous laisse choisir les skills et les CLI cibles (ou le dossier partagé `~/.agents/skills`) et n’écrase jamais un dossier du même nom. Les skills ajoutés depuis GitHub gardent leur source et proposent une vérification des mises à jour ; une mise à jour remplace le dossier par la dernière version du dépôt et place l’ancienne dans la corbeille de skills.
- Réglages › Sessions liste toutes les sessions locales de chaque fournisseur. Recherche par titre, projet ou ID, filtres par projet et par période, et page de détail avec le modèle, la répartition des tokens, le coût équivalent API, les fichiers et un aperçu de la conversation. Les sessions sélectionnées sont déplacées une à une dans la corbeille du système après confirmation, jamais supprimées définitivement ; les index et bases de données officiels restent intacts et les statistiques dérivées sont recalculées à partir des journaux restants.
- Réglages › MCP liste les serveurs MCP globaux de niveau utilisateur de chaque fournisseur (`config.toml` pour Codex et Grok, `.claude.json` pour Claude). Seul un résumé expurgé de la commande, du site et de la structure est affiché, jamais les valeurs d’env ou de headers, et Quosparr ne se connecte à aucun serveur et n’en lance aucun. Les entrées sélectionnées peuvent être supprimées ensemble : le fichier est d’abord sauvegardé dans `~/.quosparr/backups`, les autres réglages sont conservés et la modification s’applique au redémarrage de la CLI. Les serveurs Codex et Grok ont aussi un interrupteur d’activation, enregistré après la même sauvegarde : Codex écrit `enabled` sur l’entrée, et Grok met aussi à jour `disabled_mcp_servers`, comme sa propre CLI. Claude Code enregistre l’activation MCP par projet ; les serveurs Claude n’ont donc pas d’interrupteur. Les serveurs MCP de projet, de plugin ou gérés par l’entreprise ne sont pas affichés.
- Apparences claire, sombre et système ; English, 简体中文, 繁體中文, 한국어, 日本語, Русский, Français.
- Mises à jour intégrées et widgets bureau sur macOS 14+.

## Captures d’écran

<table>
  <tr>
    <th colspan="2">Panneau principal</th>
    <th colspan="2">Coût équivalent API</th>
    <th colspan="2">Estimation du quota d’abonnement</th>
  </tr>
  <tr>
    <td colspan="2" align="center" valign="top"><img src="docs/images/1-en.webp?v2" alt="Quosparr Panneau principal" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/3.webp?v1" alt="Quosparr Coût équivalent API" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/6.webp?v1" alt="Quosparr Estimation du quota d’abonnement" width="240"></td>
  </tr>
  <tr>
    <th colspan="3">Comptes fournisseurs</th>
    <th colspan="3">Réglages d’affichage</th>
  </tr>
  <tr>
    <td colspan="3" align="center" valign="top"><img src="docs/images/4.webp?v1" alt="Quosparr Comptes fournisseurs" width="370"></td>
    <td colspan="3" align="center" valign="top"><img src="docs/images/5.webp?v1" alt="Quosparr Réglages d’affichage" width="370"></td>
  </tr>
  <tr>
    <th colspan="6">Windows : réglages et panneau Grok</th>
  </tr>
  <tr>
    <td colspan="6" align="center" valign="top"><img src="docs/images/2.webp?v1" alt="Quosparr Windows : réglages et panneau Grok" width="780"></td>
  </tr>
</table>

Les captures montrent l’interface v1 sur macOS et Windows.

## Installation

Téléchargez depuis [GitHub Releases](https://github.com/Licoy/Quosparr/releases) et choisissez le paquet correspondant à votre système et à votre processeur :

| Système | Paquet | Comment l’installer |
| --- | --- | --- |
| macOS 12+ (Apple Silicon / Intel) | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg`, ou le `.zip` du même nom | Glissez `Quosparr.app` dans `Applications` |
| Windows 10 / 11 (amd64 / arm64) | `Quosparr.Setup.<version>.<arch>.exe` | Installation par utilisateur dans `%LOCALAPPDATA%\Programs\Quosparr` ; aucun droit d’administrateur requis |
| Linux (amd64 / arm64) | `quosparr-<version>-linux-<arch>.tar.gz` avec `install.sh`, ou `quosparr_<version>_<arch>.deb` | Exécutez `sh install.sh` à côté de l’archive pour installer dans `~/.local`, ou installez le deb avec votre gestionnaire de paquets |

Chaque paquet embarque l’environnement d’exécution SQLite de son architecture : le premier lancement ne télécharge donc rien. Au premier lancement, Quosparr active par défaut l’ouverture à la connexion ; modifiez-la dans **Panneau de configuration → Général → Ouvrir à la connexion**.

L’installation via Homebrew n’est pas encore disponible ; utilisez pour l’instant le DMG ou le ZIP.

Si vous utilisiez CodexRunway, consultez le [guide de migration](docs/upgrade-from-codexrunway_FR.md).

### Avertissements de sécurité macOS

Les builds macOS sont actuellement signés ad hoc et non notariés. Si macOS indique que le développeur ne peut pas être vérifié, faites un clic droit sur `Quosparr.app` et choisissez Ouvrir, ou allez dans Réglages Système → Confidentialité et sécurité et cliquez sur Ouvrir quand même.

Si macOS indique que l’app est endommagée, c’est généralement l’attribut de quarantaine du téléchargement. Après avoir déplacé l’app dans `Applications`, exécutez :

```bash
xattr -dr com.apple.quarantine /Applications/Quosparr.app
```

### Prérequis Linux

Linux nécessite GTK3, AppIndicator, un bureau prenant en charge la zone de notification et une session Secret Service fonctionnelle (par exemple GNOME Keyring ou KWallet) pour stocker les identifiants du proxy. Le deb déclare ces dépendances et se met à jour via le gestionnaire de paquets.

## Prérequis

- Codex installé et utilisé sur cette machine est recommandé. Les données Codex proviennent de `CODEX_HOME` (par défaut `~/.codex`, ou `%USERPROFILE%\.codex` sous Windows).
- Seul le magasin d’identifiants `file` est pris en charge (valeur par défaut lorsque `cli_auth_credentials_store` n’est pas défini dans le `config.toml` de Codex). `keyring`, `auto` et `ephemeral` sont signalés comme non pris en charge et bloquent l’import et le basculement de comptes.
- Le panneau Claude lit la configuration Claude Code dans `CLAUDE_CONFIG_DIR` s’il est défini, sinon dans `~/.claude`. L’usage d’abonnement exige une connexion Claude.ai ; une clé API ou une passerelle ne le fournit pas. La connexion par navigateur ne demande ni installation ni lancement préalable du CLI et reste dans la bibliothèque avant un basculement explicite. Les connexions CLI existantes sont ajoutées automatiquement ; vous pouvez aussi coller un JSON d’identifiants ou importer un fichier. L’autorisation réelle par navigateur et le basculement restent à valider avec un compte isolé.
- Le panneau Grok nécessite le [CLI Grok officiel](https://docs.x.ai/build/overview) :

  ```bash
  curl -fsSL https://x.ai/cli/install.sh | bash
  ```

- Les comptes et le quota Grok ne prennent en charge que OAuth / SuperGrok et les sessions legacy compatibles. Exécutez `grok login --oauth`, connectez-vous depuis l’app, importez la connexion actuelle ou collez un JSON d’identifiants. Les connexions par clé API seule ne sont pas ajoutées comme comptes gérés. `GROK_HOME` est utilisé s’il est défini, sinon `~/.grok`.

## Widgets bureau

Les widgets nécessitent macOS 14 ou plus ; Windows et Linux n’en disposent pas. Quatre types de widgets sont proposés : aperçu des quotas, tendance des jetons, un indicateur clé et Actualités des réinitialisations Codex. Chaque widget peut choisir Codex, Claude, Grok ou une paire : Codex + Claude, Codex + Grok, Grok + Claude. Un widget déjà réglé sur Codex + Grok conserve cette paire. Un clic ouvre la page correspondante.

L’instantané se trouve dans `~/.quosparr/widget-snapshot.json` avec les droits `0600` et ne contient que des données dérivées non secrètes : ni e-mail, ni ID de compte, ni jeton, ni JSON d’authentification, ni texte brut d’événement. L’app principale est la seule à écrire ; les widgets ne font que lire.

## Proxy réseau

Dans **Panneau de configuration → Général → Réseau**, choisissez **Réglages système**, HTTP ou SOCKS5 ; les proxys personnalisés acceptent un nom d’utilisateur et un mot de passe. Le réglage couvre les requêtes Codex, Claude et Grok lancées par l’app, les opérations CLI, les flux publics de statut et de prix, ainsi que les mises à jour du logiciel. Le proxy système n’est pas modifié, et les pages de connexion du navigateur ainsi que les CLI externes déjà lancées gardent leurs propres réglages.

Les changements ne s’appliquent qu’après **Enregistrer le proxy**. **Tester la connexion** accède au site de test (par défaut `https://github.com`, personnalisable) avec les réglages en cours de saisie, sans envoyer d’identifiants de compte ni enregistrer ces réglages. Les mots de passe du proxy sont stockés dans le trousseau macOS, le Gestionnaire d’informations d’identification Windows ou le Secret Service de Linux. Des réglages invalides, des identifiants indisponibles ou l’échec d’un proxy personnalisé ne provoquent jamais de repli sur une connexion directe.

## Confidentialité

- Quosparr conserve ses propres données dans `~/.quosparr` et n’écrit aucun cache d’application dans `~/.codex`. Par défaut, seules des données dérivées non secrètes sont stockées.
- Les identifiants des comptes fournisseurs et les profils Desktop explicitement confirmés ci-dessus sont des exceptions contrôlées ; les identifiants ils sont stockés sous `~/.quosparr/accounts/<id>/auth.json` (répertoire `0700`, fichier `0600`, écriture atomique ; sous Windows, une ACL accessible uniquement à l’utilisateur actuel). L’index des comptes ne contient aucun jeton.
- Le `~/.codex/auth.json` officiel n’est écrit que lors d’une actualisation OAuth ou lorsque vous confirmez un basculement de compte. Actualiser un compte non actuel ne met à jour que sa copie dans la bibliothèque.
- Les identifiants Claude sont lus sur macOS depuis le trousseau via `/usr/bin/security`, et sous Windows et Linux depuis `<config>/.credentials.json`. Les copies gérées sont dans `~/.quosparr/accounts/claude-<stable-id>/auth.json` et ne contiennent que `claudeAiOauth`. L’index `claude-index.json` ne contient pas de jetons. Un basculement conserve les jetons MCP et ne remplace que `oauthAccount` dans `.claude.json`. Quosparr n’actualise pas le compte actuel.
- Les clés des comptes API ne sont stockées que dans la copie gérée (`~/.quosparr/accounts/<id>/auth.json` ou `claude-api-<id>/auth.json`) ; les index ne gardent que l’URL de base, les modèles et le solde en cache. Les requêtes de solde et de modèles ne vont qu’au site saisi, via les réglages de proxy. Les fichiers `config.toml` et `settings.json` modifiés sont d’abord sauvegardés dans `~/.quosparr/backups` ; la première écriture de `settings.json` trie ses clés.
- Les identifiants Grok sont lus depuis `$GROK_HOME/auth.json` ; les copies gérées se trouvent dans `~/.quosparr/accounts/grok-<stable-id>/auth.json`. Un basculement Grok ne remplace que les portées OAuth / legacy compatibles et conserve la clé API et les portées inconnues. Il n’est garanti que pour les nouvelles sessions ; les processus Grok en cours ne sont pas arrêtés et peuvent réécrire le compte précédent, c’est pourquoi l’app avertit avant de basculer.
- La réparation des sessions ne touche que `session_index.jsonl` et en crée d’abord une sauvegarde. Les journaux de session ne partent dans la corbeille du système que si vous les sélectionnez et confirmez dans Réglages › Sessions ; chaque fichier est revérifié avant d’être déplacé, rien n’est supprimé définitivement et les index et bases de données officiels ne sont jamais réécrits.
- L’activation, la réparation et la suppression des skills et des serveurs MCP n’ont lieu qu’après votre confirmation. Les fichiers de configuration sont sauvegardés dans `~/.quosparr/backups` avant toute écriture, les skills supprimés passent d’abord par la corbeille de skills restaurable de Quosparr, et la liste MCP n’affiche jamais les clés ni les valeurs d’env et de headers.
- L’ajout d’un skill liste d’abord les fichiers de la source et ne lit en mémoire que les dossiers des skills avant d’écrire quoi que ce soit : un gros dépôt coûte du temps de téléchargement, pas de la mémoire ; leurs scripts ne sont jamais exécutés ; les sources GitHub ne contactent que GitHub en HTTPS, selon les réglages de proxy. L’origine de chaque skill ajouté (dépôt, ref, commit, dossier dans le dépôt ou chemin local) est conservée dans `~/.quosparr/skill-sources.json` (`0600`), sans aucun secret.
- Le coût équivalent API est calculé à partir des journaux de session locaux. Le contenu des sessions n’est pas téléversé.
- Les jetons et les clés API ne sont jamais écrits dans les journaux, la sortie d’auto-diagnostic ou les modèles d’issue.
- Les Actualités des réinitialisations Codex et la recherche de mises à jour ne récupèrent que des données publiques ; aucun compte, jeton ni contenu de session n’est envoyé.

## Sources de données

- **Quota Codex / reset credits / usage officiel** : API backend officielles ChatGPT / Codex, avec vos identifiants locaux. L’usage officiel appartient au compte actuel et peut être en retard ou révisé.
- **Estimation du quota hebdomadaire** : non officielle, extrapolée à partir du pourcentage hebdomadaire et des Credits journaliers (1000 Credits ≈ 40 $, version `credits-usd-2026-08-26`). Seuls les Credits et pourcentages dérivés sont enregistrés dans `quota-estimate-history.json`.
- **Coût équivalent API Codex** : journaux de session locaux valorisés avec la table intégrée `openai-builtin-2026-09-27` ; les tarifs actuels pris en charge viennent de models.dev en conservant les règles intégrées vérifiées. Les modèles inconnus ne sont pas présentés comme des coûts exacts.
- **Usage d’abonnement Claude Code** : un point d’accès OAuth usage non documenté. Il ne concerne que l’abonnement Claude Code actuel et n’est pas présenté comme une API de facturation officielle. Ni cookies du navigateur ni facture API de la Console ; les profils Desktop sont gérés uniquement par la fonction distincte et explicitement confirmée décrite ci-dessus.
- **Coût équivalent API Claude** : journaux de session locaux de Claude Code et table `anthropic-builtin-2026-10-14`. Une réponse n’est comptée qu’une fois, par identifiant de message. Les écritures de cache sont séparées en 5 minutes et 1 heure, et le tarif Fast est distinct. Les modèles inconnus n’ont pas de coût exact. Les jours suivent le fuseau local.
- **Quota Grok** : uniquement le point d’accès officiel CLI chat-proxy `/v1/billing?format=credits`. Ni cookies du navigateur, ni quota déduit des journaux locaux, ni facturation API mélangée.
- **Coût équivalent API Grok** : usage local `turn_completed` valorisé avec `xai-builtin-2026-09-27`, couvrant les tarifs d’entrée, de cache, de sortie et de long contexte. `costUsdTicks` relève de la comptabilité d’abonnement et n’est pas utilisé comme coût équivalent API.
- **Actualités des réinitialisations Codex** : le flux public [didcodexreset.com/api/status.json](https://didcodexreset.com/api/status.json). Non officiel, indicatif seulement et parfois en retard.
- Dans le graphique, « Stats officielles (tous les appareils) » appartient au compte actuel, tandis que « Journaux locaux (toutes les sessions) » analyse cette machine et peut couvrir plusieurs comptes. L’une n’est pas un sous-ensemble de l’autre et elles ne doivent pas être soustraites ; les valeurs quotidiennes Codex des deux utilisent des jours UTC.

## Développement

L’application principale est écrite en Go avec l’interface native [mygo](https://github.com/egoist/mygo) ; les versions de Go et de mygo sont figées dans `go.mod`. L’extension de widget macOS reste en Swift.

```bash
go test ./...
go vet ./...
go build ./...
```

Lancez avec un HOME temporaire et un domaine de préférences distinct afin de ne pas toucher à vos vrais comptes et réglages :

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run .
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
```

L’auto-diagnostic n’effectue aucune requête réseau et ne lit aucun identifiant. Il couvre actuellement la version, la plateforme, les droits des répertoires propres, les verrous et la présence de l’auth Codex et de l’index de session ; les autres diagnostics sont indiqués comme `not ported yet`.

Construisez un paquet de développement macOS (mise à jour désactivée ; `ARCH=arm64` ou `ARCH=x86_64` en option) :

```bash
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

Voir [docs/v1-release.md](docs/v1-release.md) pour le paquetage Windows / Linux et les vérifications de publication, et [CONTRIBUTORS.md](CONTRIBUTORS.md) pour les notes de contribution.

## Communauté

- [LinuxDO](https://linux.do/)

## Licence

Ce projet suit le [LICENSE](LICENSE) du dépôt.
